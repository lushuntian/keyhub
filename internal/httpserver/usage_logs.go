package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/newapi"
	"keyhub/internal/security"
)

const (
	defaultUsageLogPageSize = 20
	maxUsageLogPageSize     = 100
	usageLogRequestTimeout  = 20 * time.Second
	usageLogConcurrency     = 6
)

var sensitiveUsageLogContentPattern = regexp.MustCompile(
	`(?i)\b(user(?:name|_id)?|channel(?:_id|_name)?|group|token(?:_id|_name)?|proxy[_ -]?pool[_ -]?id)\b\s*[:=#]\s*("[^"]*"|'[^']*'|[^,;\s]+)`,
)

type usageLogFilter struct {
	PageSize          int    `json:"pageSize"`
	Type              int    `json:"type"`
	ModelName         string `json:"modelName"`
	StartTimestamp    int64  `json:"startTimestamp"`
	EndTimestamp      int64  `json:"endTimestamp"`
	LocalChannelID    int64  `json:"localChannelId"`
	RequestID         string `json:"requestId"`
	UpstreamRequestID string `json:"upstreamRequestId"`
}

type usageLogCursor struct {
	TargetCode string         `json:"targetCode"`
	FilterHash string         `json:"filterHash"`
	Consumed   map[string]int `json:"consumed"`
}

type usageLogItemResponse struct {
	CreatedAt                int64   `json:"createdAt"`
	Type                     int     `json:"type"`
	LocalChannelID           int64   `json:"channelId"`
	ModelName                string  `json:"modelName"`
	Quota                    int64   `json:"quota"`
	QuotaUSD                 float64 `json:"quotaUsd"`
	PromptTokens             int64   `json:"promptTokens"`
	CompletionTokens         int64   `json:"completionTokens"`
	CacheReadTokens          int64   `json:"cacheReadTokens"`
	CacheCreationTokens      int64   `json:"cacheCreationTokens"`
	UseTime                  int64   `json:"useTime"`
	FirstResponseTimeMS      int64   `json:"firstResponseTimeMs"`
	IsStream                 bool    `json:"isStream"`
	RequestID                string  `json:"requestId,omitempty"`
	UpstreamRequestID        string  `json:"upstreamRequestId,omitempty"`
	Content                  string  `json:"content,omitempty"`
	StreamStatus             string  `json:"streamStatus,omitempty"`
	StreamEndReason          string  `json:"streamEndReason,omitempty"`
	UpstreamUsageLogSequence int64   `json:"-"`
}

type usageLogStatsResponse struct {
	Quota    int64   `json:"quota"`
	QuotaUSD float64 `json:"quotaUsd"`
	RPM      int64   `json:"rpm"`
	TPM      int64   `json:"tpm"`
}

type usageLogPageResponse struct {
	Configured bool                   `json:"configured"`
	Message    string                 `json:"message,omitempty"`
	Items      []usageLogItemResponse `json:"items"`
	Total      int64                  `json:"total"`
	PageSize   int                    `json:"pageSize"`
	NextCursor string                 `json:"nextCursor,omitempty"`
	Channels   []int64                `json:"channels"`
	Stats      usageLogStatsResponse  `json:"stats"`
}

type usageLogConfigurationResponse struct {
	Configured bool   `json:"configured"`
	Message    string `json:"message,omitempty"`
}

type usageLogChannelResult struct {
	Mapping database.UsageLogChannelMapping
	Items   []newapi.UsageLog
	Total   int64
	Stats   newapi.UsageLogStats
	Err     error
}

type mappedUsageLog struct {
	Mapping database.UsageLogChannelMapping
	Log     newapi.UsageLog
}

func (s *Server) handleUsageLogConfiguration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), usageLogRequestTimeout)
	defer cancel()

	_, configuredMessage, err := s.systemDefaultReverseTarget(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, usageLogConfigurationResponse{
		Configured: configuredMessage == "",
		Message:    configuredMessage,
	})
}

func (s *Server) handleUsageLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter, cursor, err := parseUsageLogRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), usageLogRequestTimeout)
	defer cancel()

	target, configuredMessage, err := s.systemDefaultReverseTarget(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if configuredMessage != "" {
		writeJSON(w, http.StatusOK, usageLogPageResponse{
			Configured: false,
			Message:    configuredMessage,
			Items:      []usageLogItemResponse{},
			PageSize:   filter.PageSize,
			Channels:   []int64{},
		})
		return
	}

	owners, err := s.keyOwnerFilters(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	mappings, err := database.ListUsageLogChannelMappings(ctx, s.db, target.Code, owners)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	channels := usageLogLocalChannelIDs(mappings)
	mappings = filterUsageLogMappings(mappings, filter.LocalChannelID)
	if len(mappings) == 0 {
		writeJSON(w, http.StatusOK, usageLogPageResponse{
			Configured: true,
			Items:      []usageLogItemResponse{},
			PageSize:   filter.PageSize,
			Channels:   channels,
		})
		return
	}

	filterHash, err := hashUsageLogFilter(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cursor.TargetCode != "" && (cursor.TargetCode != target.Code || cursor.FilterHash != filterHash) {
		writeError(w, http.StatusBadRequest, "使用日志分页游标已失效，请重新查询")
		return
	}
	if cursor.Consumed == nil {
		cursor.Consumed = make(map[string]int)
	}

	client, err := s.reverseClientForTarget(r.Context(), target)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	results := fetchUsageLogsByChannels(ctx, client, mappings, filter, cursor.Consumed)
	candidates := make([]mappedUsageLog, 0, len(mappings)*filter.PageSize)
	var total int64
	var stats newapi.UsageLogStats
	for _, result := range results {
		if result.Err != nil {
			writeError(w, http.StatusBadGateway, result.Err.Error())
			return
		}
		total += result.Total
		stats.Quota += result.Stats.Quota
		stats.RPM += result.Stats.RPM
		stats.TPM += result.Stats.TPM
		for _, log := range result.Items {
			candidates = append(candidates, mappedUsageLog{Mapping: result.Mapping, Log: log})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Log.CreatedAt != candidates[j].Log.CreatedAt {
			return candidates[i].Log.CreatedAt > candidates[j].Log.CreatedAt
		}
		if candidates[i].Log.ID != candidates[j].Log.ID {
			return candidates[i].Log.ID > candidates[j].Log.ID
		}
		return candidates[i].Mapping.RemoteChannelID > candidates[j].Mapping.RemoteChannelID
	})
	if len(candidates) > filter.PageSize {
		candidates = candidates[:filter.PageSize]
	}

	items := make([]usageLogItemResponse, 0, len(candidates))
	nextConsumed := cloneUsageLogConsumed(cursor.Consumed)
	for _, candidate := range candidates {
		key := strconv.FormatInt(candidate.Mapping.RemoteChannelID, 10)
		nextConsumed[key]++
		items = append(items, sanitizeUsageLog(candidate.Log, candidate.Mapping.LocalChannelID, s.config.NewAPIQuotaPerUSD))
	}
	nextCursor := ""
	if usageLogHasMore(mappings, results, nextConsumed) {
		nextCursor, err = encodeUsageLogCursor(usageLogCursor{
			TargetCode: target.Code,
			FilterHash: filterHash,
			Consumed:   nextConsumed,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, usageLogPageResponse{
		Configured: true,
		Items:      items,
		Total:      total,
		PageSize:   filter.PageSize,
		NextCursor: nextCursor,
		Channels:   channels,
		Stats: usageLogStatsResponse{
			Quota:    stats.Quota,
			QuotaUSD: quotaToUSD(stats.Quota, s.config.NewAPIQuotaPerUSD),
			RPM:      stats.RPM,
			TPM:      stats.TPM,
		},
	})
}

func (s *Server) systemDefaultReverseTarget(ctx context.Context) (config.AggregationTarget, string, error) {
	records, err := database.ListAggregationTargetSecrets(ctx, s.db, false)
	if err != nil {
		return config.AggregationTarget{}, "", err
	}
	var record *database.AggregationTargetRecord
	for index := range records {
		if records[index].IsSystemDefault {
			record = &records[index]
			break
		}
	}
	if record == nil {
		return config.AggregationTarget{}, "请先在“接收平台”中设置系统默认接收平台", nil
	}
	if !record.Enabled {
		return config.AggregationTarget{}, "系统默认接收平台当前已停用", nil
	}
	if normalizeAggregationTargetConnectionModeOrAPI(record.ConnectionMode) != aggregationTargetConnectionModeNewAPIReverse {
		return config.AggregationTarget{}, "系统默认接收平台必须使用 new-api逆向 接入方式", nil
	}
	if strings.TrimSpace(record.ReverseUsername) == "" || strings.TrimSpace(record.ReversePasswordCiphertext) == "" {
		return config.AggregationTarget{}, "系统默认接收平台缺少 new-api 逆向账号或密码", nil
	}
	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		return config.AggregationTarget{}, "", err
	}
	password, err := encryptor.Decrypt(record.ReversePasswordCiphertext)
	if err != nil {
		return config.AggregationTarget{}, "", fmt.Errorf("decrypt system default aggregation target password: %w", err)
	}
	return config.AggregationTarget{
		Code:             record.Code,
		Name:             record.Name,
		BaseURL:          record.BaseURL,
		ConnectionMode:   aggregationTargetConnectionModeNewAPIReverse,
		ReverseUsername:  record.ReverseUsername,
		ReversePassword:  password,
		ReverseGroupName: record.ReverseGroupName,
	}, "", nil
}

func parseUsageLogRequest(r *http.Request) (usageLogFilter, usageLogCursor, error) {
	query := r.URL.Query()
	pageSize, err := parseBoundedQueryInt(query.Get("pageSize"), defaultUsageLogPageSize, 1, maxUsageLogPageSize, "pageSize")
	if err != nil {
		return usageLogFilter{}, usageLogCursor{}, err
	}
	logType, err := parseBoundedQueryInt(query.Get("type"), 0, 0, 7, "type")
	if err != nil {
		return usageLogFilter{}, usageLogCursor{}, err
	}
	startTimestamp, err := parseNonNegativeQueryInt64(query.Get("startTimestamp"), "startTimestamp")
	if err != nil {
		return usageLogFilter{}, usageLogCursor{}, err
	}
	endTimestamp, err := parseNonNegativeQueryInt64(query.Get("endTimestamp"), "endTimestamp")
	if err != nil {
		return usageLogFilter{}, usageLogCursor{}, err
	}
	if startTimestamp > 0 && endTimestamp > 0 && startTimestamp > endTimestamp {
		return usageLogFilter{}, usageLogCursor{}, errors.New("startTimestamp must be less than or equal to endTimestamp")
	}
	localChannelID, err := parseNonNegativeQueryInt64(query.Get("channelId"), "channelId")
	if err != nil {
		return usageLogFilter{}, usageLogCursor{}, err
	}
	filter := usageLogFilter{
		PageSize:          pageSize,
		Type:              logType,
		ModelName:         cleanUsageLogQueryText(query.Get("modelName")),
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
		LocalChannelID:    localChannelID,
		RequestID:         cleanUsageLogQueryText(query.Get("requestId")),
		UpstreamRequestID: cleanUsageLogQueryText(query.Get("upstreamRequestId")),
	}
	for label, value := range map[string]string{
		"modelName":         filter.ModelName,
		"requestId":         filter.RequestID,
		"upstreamRequestId": filter.UpstreamRequestID,
	} {
		if len([]rune(value)) > 128 {
			return usageLogFilter{}, usageLogCursor{}, fmt.Errorf("%s must be 128 characters or fewer", label)
		}
	}
	cursor, err := decodeUsageLogCursor(query.Get("cursor"))
	if err != nil {
		return usageLogFilter{}, usageLogCursor{}, err
	}
	return filter, cursor, nil
}

func fetchUsageLogsByChannels(
	ctx context.Context,
	client *newapi.ReverseClient,
	mappings []database.UsageLogChannelMapping,
	filter usageLogFilter,
	consumed map[string]int,
) []usageLogChannelResult {
	results := make([]usageLogChannelResult, len(mappings))
	semaphore := make(chan struct{}, usageLogConcurrency)
	var wg sync.WaitGroup
	for index, mapping := range mappings {
		wg.Add(1)
		go func(index int, mapping database.UsageLogChannelMapping) {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results[index] = usageLogChannelResult{Mapping: mapping, Err: ctx.Err()}
				return
			}
			offset := consumed[strconv.FormatInt(mapping.RemoteChannelID, 10)]
			items, total, err := fetchUsageLogWindow(ctx, client, mapping.RemoteChannelID, offset, filter)
			if err != nil {
				results[index] = usageLogChannelResult{Mapping: mapping, Err: err}
				return
			}
			stats, err := client.GetUsageLogStats(ctx, newAPIUsageLogQuery(filter, mapping.RemoteChannelID, 0))
			results[index] = usageLogChannelResult{
				Mapping: mapping,
				Items:   items,
				Total:   total,
				Stats:   stats,
				Err:     err,
			}
		}(index, mapping)
	}
	wg.Wait()
	return results
}

func fetchUsageLogWindow(
	ctx context.Context,
	client *newapi.ReverseClient,
	remoteChannelID int64,
	offset int,
	filter usageLogFilter,
) ([]newapi.UsageLog, int64, error) {
	if offset < 0 {
		offset = 0
	}
	page := offset/filter.PageSize + 1
	skip := offset % filter.PageSize
	first, err := client.ListUsageLogs(ctx, newAPIUsageLogQuery(filter, remoteChannelID, page))
	if err != nil {
		return nil, 0, err
	}
	items := append([]newapi.UsageLog(nil), first.Items...)
	if skip > len(items) {
		skip = len(items)
	}
	items = items[skip:]
	if len(items) < filter.PageSize && int64(offset+len(items)) < first.Total {
		next, err := client.ListUsageLogs(ctx, newAPIUsageLogQuery(filter, remoteChannelID, page+1))
		if err != nil {
			return nil, 0, err
		}
		items = append(items, next.Items...)
	}
	if len(items) > filter.PageSize {
		items = items[:filter.PageSize]
	}
	return items, first.Total, nil
}

func newAPIUsageLogQuery(filter usageLogFilter, remoteChannelID int64, page int) newapi.UsageLogQuery {
	return newapi.UsageLogQuery{
		Page:              page,
		PageSize:          filter.PageSize,
		Type:              filter.Type,
		ModelName:         filter.ModelName,
		StartTimestamp:    filter.StartTimestamp,
		EndTimestamp:      filter.EndTimestamp,
		ChannelID:         remoteChannelID,
		RequestID:         filter.RequestID,
		UpstreamRequestID: filter.UpstreamRequestID,
	}
}

func sanitizeUsageLog(log newapi.UsageLog, localChannelID int64, quotaPerUSD float64) usageLogItemResponse {
	other := parseUsageLogOther(log.Other)
	cacheCreationTokens := jsonInt64(other, "cache_creation_tokens")
	if split := jsonInt64(other, "cache_creation_tokens_5m") + jsonInt64(other, "cache_creation_tokens_1h"); split > 0 {
		cacheCreationTokens = split
	}
	streamStatus, streamEndReason := safeStreamStatus(other)
	return usageLogItemResponse{
		CreatedAt:                log.CreatedAt,
		Type:                     log.Type,
		LocalChannelID:           localChannelID,
		ModelName:                strings.TrimSpace(log.ModelName),
		Quota:                    log.Quota,
		QuotaUSD:                 quotaToUSD(log.Quota, quotaPerUSD),
		PromptTokens:             log.PromptTokens,
		CompletionTokens:         log.CompletionTokens,
		CacheReadTokens:          jsonInt64(other, "cache_tokens"),
		CacheCreationTokens:      cacheCreationTokens,
		UseTime:                  log.UseTime,
		FirstResponseTimeMS:      jsonInt64(other, "frt"),
		IsStream:                 log.IsStream,
		RequestID:                trimUsageLogText(log.RequestID, 128),
		UpstreamRequestID:        trimUsageLogText(log.UpstreamRequestID, 128),
		Content:                  redactUsageLogContent(log.Content),
		StreamStatus:             streamStatus,
		StreamEndReason:          streamEndReason,
		UpstreamUsageLogSequence: log.ID,
	}
}

func parseUsageLogOther(raw string) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage)
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return make(map[string]json.RawMessage)
	}
	return result
}

func jsonInt64(values map[string]json.RawMessage, key string) int64 {
	raw, ok := values[key]
	if !ok {
		return 0
	}
	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		return integer
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return int64(number)
	}
	return 0
}

func safeStreamStatus(values map[string]json.RawMessage) (string, string) {
	raw, ok := values["stream_status"]
	if !ok {
		return "", ""
	}
	var value struct {
		Status    string `json:"status"`
		EndReason string `json:"end_reason"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", ""
	}
	return trimUsageLogText(value.Status, 64), trimUsageLogText(value.EndReason, 128)
}

func usageLogHasMore(
	mappings []database.UsageLogChannelMapping,
	results []usageLogChannelResult,
	consumed map[string]int,
) bool {
	totalByRemote := make(map[int64]int64, len(results))
	for _, result := range results {
		totalByRemote[result.Mapping.RemoteChannelID] = result.Total
	}
	for _, mapping := range mappings {
		if int64(consumed[strconv.FormatInt(mapping.RemoteChannelID, 10)]) < totalByRemote[mapping.RemoteChannelID] {
			return true
		}
	}
	return false
}

func filterUsageLogMappings(mappings []database.UsageLogChannelMapping, localChannelID int64) []database.UsageLogChannelMapping {
	if localChannelID <= 0 {
		return mappings
	}
	filtered := make([]database.UsageLogChannelMapping, 0, 1)
	for _, mapping := range mappings {
		if mapping.LocalChannelID == localChannelID {
			filtered = append(filtered, mapping)
		}
	}
	return filtered
}

func usageLogLocalChannelIDs(mappings []database.UsageLogChannelMapping) []int64 {
	seen := make(map[int64]struct{}, len(mappings))
	items := make([]int64, 0, len(mappings))
	for _, mapping := range mappings {
		if _, ok := seen[mapping.LocalChannelID]; ok {
			continue
		}
		seen[mapping.LocalChannelID] = struct{}{}
		items = append(items, mapping.LocalChannelID)
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	return items
}

func hashUsageLogFilter(filter usageLogFilter) (string, error) {
	payload, err := json.Marshal(filter)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func encodeUsageLogCursor(cursor usageLogCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeUsageLogCursor(raw string) (usageLogCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return usageLogCursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return usageLogCursor{}, errors.New("invalid usage log cursor")
	}
	var cursor usageLogCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return usageLogCursor{}, errors.New("invalid usage log cursor")
	}
	if len(cursor.Consumed) > 10000 {
		return usageLogCursor{}, errors.New("invalid usage log cursor")
	}
	for key, value := range cursor.Consumed {
		if _, err := strconv.ParseInt(key, 10, 64); err != nil || value < 0 || value > 1_000_000_000 {
			return usageLogCursor{}, errors.New("invalid usage log cursor")
		}
	}
	return cursor, nil
}

func cloneUsageLogConsumed(values map[string]int) map[string]int {
	cloned := make(map[string]int, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func parseBoundedQueryInt(raw string, fallback int, minimum int, maximum int, label string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", label, minimum, maximum)
	}
	return value, nil
}

func parseNonNegativeQueryInt64(raw string, label string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", label)
	}
	return value, nil
}

func cleanUsageLogQueryText(value string) string {
	return strings.TrimSpace(value)
}

func trimUsageLogText(value string, maximum int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum]) + "…"
}

func redactUsageLogContent(value string) string {
	value = sensitiveUsageLogContentPattern.ReplaceAllString(value, "$1=[已隐藏]")
	return trimUsageLogText(value, 2048)
}

func quotaToUSD(quota int64, quotaPerUSD float64) float64 {
	if quotaPerUSD <= 0 {
		return 0
	}
	return float64(quota) / quotaPerUSD
}
