package httpserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/keys"
	"keyhub/internal/security"
)

const maxAPIImportKeys = 100
const maxAPIImportTasks = 500

var apiIdempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,128}$`)
var apiJobIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type publicKeyImportRequest struct {
	Keys              []string `json:"keys"`
	ChannelTypes      []string `json:"channelTypes"`
	Models            []string `json:"models,omitempty"`
	AWSCredentialType string   `json:"awsCredentialType,omitempty"`
	AWSBedrockRegions []string `json:"awsBedrockRegions,omitempty"`
}

func readAPIJSON(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("only one JSON object is allowed")
	}
	return nil
}

func normalizePublicImport(request *publicKeyImportRequest) ([]keys.ParsedKey, error) {
	if len(request.Keys) < 1 || len(request.Keys) > maxAPIImportKeys {
		return nil, fmt.Errorf("keys 必须包含 1–%d 个密钥", maxAPIImportKeys)
	}
	if len(request.ChannelTypes) < 1 || len(request.ChannelTypes) > 6 {
		return nil, errors.New("channelTypes 必须显式指定 1–6 种渠道类型")
	}
	legacy, modern := false, false
	seen := map[string]bool{}
	for i, channelType := range request.ChannelTypes {
		if strings.TrimSpace(channelType) == "" {
			return nil, errors.New("channelTypes 不能包含空值")
		}
		normalized, err := normalizeAggregationTargetChannelType(channelType)
		if err != nil || seen[normalized] {
			return nil, errors.New("channelTypes 包含未知或重复的渠道类型")
		}
		request.ChannelTypes[i] = normalized
		seen[normalized] = true
		legacy = legacy || normalized == aggregationTargetChannelTypeAWSBedrockLegacy
		modern = modern || normalized == aggregationTargetChannelTypeAWSBedrockNew
	}
	if modern && len(request.ChannelTypes) != 1 {
		return nil, errors.New("新版 AWS Bedrock 必须单独提交")
	}
	var err error
	request.AWSCredentialType, err = normalizeAWSCredentialType(request.AWSCredentialType)
	if err != nil {
		return nil, err
	}
	if legacy && request.AWSCredentialType == "" {
		return nil, errors.New("旧版 AWS Bedrock 必须指定 awsCredentialType")
	}
	if !legacy && request.AWSCredentialType != "" {
		return nil, errors.New("awsCredentialType 仅适用于旧版 AWS Bedrock")
	}
	if !modern && len(request.AWSBedrockRegions) > 0 {
		return nil, errors.New("awsBedrockRegions 仅适用于新版 AWS Bedrock")
	}
	if modern {
		if len(request.AWSBedrockRegions) > 20 {
			return nil, errors.New("最多支持 20 个区域地址")
		}
		request.AWSBedrockRegions, err = normalizeAWSBedrockNewRegions(request.AWSBedrockRegions)
		if err != nil {
			return nil, err
		}
		for _, region := range request.AWSBedrockRegions {
			if len(region) > 512 {
				return nil, errors.New("区域地址过长")
			}
		}
		if len(request.Keys)*len(request.AWSBedrockRegions) > maxAPIImportKeys {
			return nil, fmt.Errorf("区域展开后最多 %d 条密钥", maxAPIImportKeys)
		}
	}
	if len(request.Models) > 200 {
		return nil, errors.New("最多支持 200 个模型")
	}
	for _, model := range request.Models {
		if len(model) > 128 {
			return nil, errors.New("模型标识最长 128 字节")
		}
	}
	request.Models = cleanModelList(request.Models)
	items := make([]keys.ParsedKey, 0, len(request.Keys))
	for i, key := range request.Keys {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 8192 || strings.ContainsAny(key, "\r\n\x00") {
			return nil, fmt.Errorf("keys[%d] 必须是 1–8192 字节的单行密钥", i)
		}
		request.Keys[i] = key
		item := keys.ParseLines(key)[0]
		item.LineNumber = i + 1
		items = append(items, item)
	}
	if legacy {
		if err := validateAWSCredentialLines(items, request.AWSCredentialType); err != nil {
			return nil, err
		}
	}
	return expandParsedKeysByBaseURLs(items, request.AWSBedrockRegions), nil
}

func publicImportHash(request publicKeyImportRequest, secret string) string {
	data, _ := json.Marshal(request)
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

func apiTargetSignature(target config.AggregationTarget) string {
	username := ""
	mode := normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode)
	if mode != aggregationTargetConnectionModeAPI {
		username = target.ReverseUsername
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{strings.TrimRight(target.BaseURL, "/"), mode, normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType), username}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (s *Server) handlePublicKeyOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, 405, "METHOD_NOT_ALLOWED", "仅支持 GET")
		return
	}
	records, err := database.ListUserAggregationTargets(r.Context(), s.db, currentAdminUser(r).Username)
	if err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "读取渠道选项失败")
		return
	}
	writeJSON(w, 200, map[string]any{"items": regularUserAggregationTargetResponses(records), "limits": map[string]int{"maxKeys": maxAPIImportKeys, "maxExpandedKeys": maxAPIImportKeys, "maxPublishTasks": maxAPIImportTasks, "readRequestsPerMinute": 120, "writeRequestsPerMinute": 10}, "publishingEnabled": s.config.WorkerEnabled})
}

func (s *Server) handlePublicKeyImports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, 405, "METHOD_NOT_ALLOWED", "仅支持 POST")
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if !apiIdempotencyPattern.MatchString(idempotencyKey) {
		apiError(w, 400, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key 须为 8–128 位字母、数字或 _ . : -")
		return
	}
	var request publicKeyImportRequest
	if err := readAPIJSON(w, r, &request); err != nil {
		apiError(w, 400, "INVALID_REQUEST", "请提交单个合法 JSON 对象，检查字段名称和 2 MiB 请求上限")
		return
	}
	items, err := normalizePublicImport(&request)
	if err != nil {
		apiError(w, 400, "INVALID_REQUEST", err.Error())
		return
	}
	user := currentAdminUser(r)
	hash := publicImportHash(request, s.config.EncryptionKey)
	release, locked, err := database.AcquireAPILock(r.Context(), s.db, fmt.Sprintf("import:%d:%s", user.ID, idempotencyKey), 5)
	if err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "导入服务暂不可用")
		return
	}
	if !locked {
		w.Header().Set("Retry-After", "2")
		apiError(w, 409, "REQUEST_IN_PROGRESS", "相同请求正在处理中，请使用相同 Idempotency-Key 重试")
		return
	}
	defer release()
	existingID, existingHash, err := database.FindAPIImportRequest(r.Context(), s.db, user.ID, idempotencyKey)
	if err == nil {
		if existingHash != hash {
			apiError(w, 409, "IDEMPOTENCY_CONFLICT", "此 Idempotency-Key 已用于不同的请求")
			return
		}
		s.writePublicImport(w, r, existingID, 200)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		apiError(w, 500, "INTERNAL_ERROR", "查询导入请求失败")
		return
	}
	if !s.config.WorkerEnabled {
		apiError(w, 503, "PUBLISHING_DISABLED", "后台发布任务未启用，请联系管理员")
		return
	}
	targets, err := s.enabledAggregationTargetsForOwner(r.Context(), user.Username)
	if err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "读取用户权限失败")
		return
	}
	targets, err = filterAggregationTargetsByChannelTypes(targets, request.ChannelTypes)
	if err != nil || len(targets) == 0 {
		apiError(w, 403, "CHANNEL_NOT_ALLOWED", "用户未关联所选渠道类型的已启用平台")
		return
	}
	if len(items)*len(targets) > maxAPIImportTasks {
		apiError(w, 400, "BATCH_TOO_LARGE", "展开后的发布任务超过 500 个，请减少本批密钥数量")
		return
	}
	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		apiError(w, 503, "ENCRYPTION_UNAVAILABLE", "密钥加密服务不可用")
		return
	}
	rows := make([]database.APIImportRow, 0, len(items))
	for _, item := range items {
		encrypted, err := encryptor.Encrypt(item.Normalized)
		if err != nil {
			apiError(w, 500, "INTERNAL_ERROR", "密钥加密失败")
			return
		}
		hint := []rune(item.KeyHint)
		if len(hint) > 64 {
			hint = hint[:64]
		}
		rows = append(rows, database.APIImportRow{
			InputIndex: item.LineNumber - 1,
			Key: database.ImportAPIKey{
				Ciphertext: encrypted, Fingerprint: item.Fingerprint, KeyHint: string(hint),
				BaseURL: item.BaseURL, Models: request.Models, AWSCredentialType: request.AWSCredentialType,
				GroupName: "default", Status: "inventory",
			},
		})
	}
	destinations := make([]database.APIImportTarget, 0, len(targets))
	for _, target := range targets {
		destinations = append(destinations, database.APIImportTarget{Code: target.Code, ChannelType: target.ChannelType, Signature: apiTargetSignature(target)})
	}
	rawID := make([]byte, 16)
	if _, err := rand.Read(rawID); err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "生成批次失败")
		return
	}
	jobID := hex.EncodeToString(rawID)
	tokenID, _ := r.Context().Value(apiTokenContextKey).(int64)
	if err := database.EnqueueAPIImport(r.Context(), s.db, jobID, user, tokenID, idempotencyKey, hash, rows, destinations); err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "导入失败，请使用相同 Idempotency-Key 重试")
		return
	}
	s.writePublicImport(w, r, jobID, 202)
}

func (s *Server) handlePublicKeyImportAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/key-imports/"), "/")
	if len(parts) < 1 || len(parts) > 2 || !apiJobIDPattern.MatchString(parts[0]) {
		apiError(w, 404, "NOT_FOUND", "批次不存在")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			apiError(w, 405, "METHOD_NOT_ALLOWED", "仅支持 GET")
			return
		}
		s.writePublicImport(w, r, parts[0], 200)
		return
	}
	if parts[1] != "retry" {
		apiError(w, 404, "NOT_FOUND", "接口不存在")
		return
	}
	if r.Method != http.MethodPost {
		apiError(w, 405, "METHOD_NOT_ALLOWED", "仅支持 POST")
		return
	}
	if !s.config.WorkerEnabled {
		apiError(w, 503, "PUBLISHING_DISABLED", "后台发布任务未启用")
		return
	}
	user := currentAdminUser(r)
	tokenID, _ := r.Context().Value(apiTokenContextKey).(int64)
	_, err := database.RetryAPIImport(r.Context(), s.db, user.ID, tokenID, parts[0])
	if errors.Is(err, sql.ErrNoRows) {
		apiError(w, 404, "NOT_FOUND", "批次不存在")
		return
	}
	if err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "重试失败")
		return
	}
	s.writePublicImport(w, r, parts[0], 202)
}

func (s *Server) writePublicImport(w http.ResponseWriter, r *http.Request, id string, status int) {
	job, err := database.LoadAPIImportJob(r.Context(), s.db, currentAdminUser(r).ID, id)
	if errors.Is(err, sql.ErrNoRows) {
		apiError(w, 404, "NOT_FOUND", "批次不存在")
		return
	}
	if err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "读取批次失败，请使用相同 Idempotency-Key 重试或查询批次")
		return
	}
	writeJSON(w, status, job)
}
