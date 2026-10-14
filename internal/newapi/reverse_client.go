package newapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ReverseAdminCheckResult struct {
	UserID       int   `json:"userId"`
	Role         int   `json:"role"`
	ChannelTotal int64 `json:"channelTotal"`
}

type UsageLogQuery struct {
	Page              int
	PageSize          int
	Type              int
	ModelName         string
	StartTimestamp    int64
	EndTimestamp      int64
	ChannelID         int64
	RequestID         string
	UpstreamRequestID string
}

type UsageLog struct {
	ID                int64  `json:"id"`
	CreatedAt         int64  `json:"created_at"`
	Type              int    `json:"type"`
	Content           string `json:"content"`
	ModelName         string `json:"model_name"`
	Quota             int64  `json:"quota"`
	PromptTokens      int64  `json:"prompt_tokens"`
	CompletionTokens  int64  `json:"completion_tokens"`
	UseTime           int64  `json:"use_time"`
	IsStream          bool   `json:"is_stream"`
	ChannelID         int64  `json:"channel"`
	RequestID         string `json:"request_id"`
	UpstreamRequestID string `json:"upstream_request_id"`
	Other             string `json:"other"`
}

type UsageLogPage struct {
	Items    []UsageLog `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

type UsageLogStats struct {
	Quota int64 `json:"quota"`
	RPM   int64 `json:"rpm"`
	TPM   int64 `json:"tpm"`
}

type ReverseClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client

	authMu      sync.Mutex
	authData    reverseLoginData
	authVersion uint64
}

type reverseLoginData struct {
	ID                             int                   `json:"id"`
	Role                           int                   `json:"role"`
	AccessToken                    string                `json:"access_token"`
	User                           *reverseLoginUserData `json:"user"`
	Session                        *reverseLoginSession  `json:"session"`
	Require2FA                     bool                  `json:"require_2fa"`
	RequireRootEmailVerification   bool                  `json:"require_root_email_verification"`
	RootLoginVerificationEmailList []any                 `json:"root_login_verification_emails"`
}

type reverseLoginSession struct {
	SID string `json:"sid"`
}

type reverseLoginUserData struct {
	ID                           int  `json:"id"`
	Role                         int  `json:"role"`
	Require2FA                   bool `json:"require_2fa"`
	RequireRootEmailVerification bool `json:"require_root_email_verification"`
}

type reverseChannelIDData struct {
	ID              int64                  `json:"id"`
	ChannelID       int64                  `json:"channel_id"`
	ChannelIDCamel  int64                  `json:"channelId"`
	Channel         *reverseChannelIDData  `json:"channel"`
	CreatedChannel  *reverseChannelIDData  `json:"createdChannel"`
	CreatedChannels []reverseChannelIDData `json:"createdChannels"`
}

type reverseHTTPStatusError struct {
	statusCode int
}

func (e reverseHTTPStatusError) Error() string {
	return fmt.Sprintf("new-api reverse http status %d", e.statusCode)
}

func NewReverseClient(baseURL string, username string, password string) (*ReverseClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("new-api 站点地址不能为空")
	}
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("new-api 账号不能为空")
	}
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("new-api 密码不能为空")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create new-api cookie jar: %w", err)
	}
	return &ReverseClient{
		baseURL:  baseURL,
		username: strings.TrimSpace(username),
		password: strings.TrimSpace(password),
		client: &http.Client{
			Jar:     jar,
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (c *ReverseClient) CheckAdminChannelList(ctx context.Context) (ReverseAdminCheckResult, error) {
	total, err := c.checkChannelList(ctx)
	if err != nil {
		return ReverseAdminCheckResult{}, err
	}
	login, _, err := c.ensureLogin(ctx)
	if err != nil {
		return ReverseAdminCheckResult{}, err
	}
	return ReverseAdminCheckResult{
		UserID:       login.ID,
		Role:         login.Role,
		ChannelTotal: total,
	}, nil
}

// Close releases a modern new-api dashboard login session. Legacy new-api
// responses do not include an access token, so there is no session to revoke
// through the modern logout endpoint in that case.
func (c *ReverseClient) Close(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	login := c.authData
	c.authData = reverseLoginData{}
	c.authVersion++
	if strings.TrimSpace(login.AccessToken) == "" {
		return nil
	}

	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/user/auth/logout", nil, login.ID, login.AccessToken, login.sessionID(), &envelope); err != nil {
		return fmt.Errorf("logout new-api reverse session: %w", err)
	}
	if !envelope.Success {
		return fmt.Errorf("logout new-api reverse session failed: %s", envelope.Message)
	}
	return nil
}

func (d reverseLoginData) sessionID() string {
	if d.Session == nil {
		return ""
	}
	return strings.TrimSpace(d.Session.SID)
}

func (c *ReverseClient) CreateChannel(ctx context.Context, payload ChannelPayload) (PushChannelResponse, error) {
	request := AddChannelRequest{
		Mode:    "single",
		Channel: &payload,
	}
	var envelope apiEnvelope
	if err := c.doAuthenticatedJSON(ctx, http.MethodPost, "/api/channel/", request, &envelope); err != nil {
		return PushChannelResponse{}, err
	}
	if !envelope.Success {
		return PushChannelResponse{}, fmt.Errorf("new-api reverse create channel failed: %s", envelope.Message)
	}
	channelID, ok, err := decodeReverseCreateChannelID(envelope.Data)
	if err != nil {
		return PushChannelResponse{}, err
	}
	if !ok {
		channel, err := c.searchChannelByName(ctx, payload.Name)
		if err != nil {
			return PushChannelResponse{}, err
		}
		channelID = channel.ID
	}
	if channelID <= 0 {
		return PushChannelResponse{}, fmt.Errorf("new-api reverse create channel returned invalid channel id")
	}
	return PushChannelResponse{
		ChannelID: channelID,
		Action:    "created",
	}, nil
}

func (c *ReverseClient) ListChannelUsage(ctx context.Context) ([]ChannelUsage, error) {
	channels, err := c.listChannels(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ChannelUsage, 0, len(channels))
	for _, channel := range channels {
		if channel.ID <= 0 {
			return nil, fmt.Errorf("new-api reverse usage returned invalid channel id")
		}
		if channel.UsedQuota < 0 {
			return nil, fmt.Errorf("new-api reverse usage returned negative used_quota for channel %d", channel.ID)
		}
		items = append(items, ChannelUsage{
			ChannelID: channel.ID,
			UsedQuota: channel.UsedQuota,
			Status:    channel.Status,
		})
	}
	return items, nil
}

func (c *ReverseClient) ListUsageLogs(ctx context.Context, query UsageLogQuery) (UsageLogPage, error) {
	values := usageLogQueryValues(query, true)
	var envelope apiEnvelope
	if err := c.doAuthenticatedJSON(ctx, http.MethodGet, "/api/log/?"+values.Encode(), nil, &envelope); err != nil {
		return UsageLogPage{}, err
	}
	if !envelope.Success {
		return UsageLogPage{}, fmt.Errorf("new-api reverse list usage logs failed: %s", envelope.Message)
	}
	var page UsageLogPage
	if err := json.Unmarshal(envelope.Data, &page); err != nil {
		return UsageLogPage{}, fmt.Errorf("decode new-api reverse usage logs: %w", err)
	}
	if page.Items == nil {
		page.Items = []UsageLog{}
	}
	return page, nil
}

func (c *ReverseClient) GetUsageLogStats(ctx context.Context, query UsageLogQuery) (UsageLogStats, error) {
	values := usageLogQueryValues(query, false)
	var envelope apiEnvelope
	if err := c.doAuthenticatedJSON(ctx, http.MethodGet, "/api/log/stat?"+values.Encode(), nil, &envelope); err != nil {
		return UsageLogStats{}, err
	}
	if !envelope.Success {
		return UsageLogStats{}, fmt.Errorf("new-api reverse get usage log stats failed: %s", envelope.Message)
	}
	var stats UsageLogStats
	if err := json.Unmarshal(envelope.Data, &stats); err != nil {
		return UsageLogStats{}, fmt.Errorf("decode new-api reverse usage log stats: %w", err)
	}
	return stats, nil
}

func usageLogQueryValues(query UsageLogQuery, includePage bool) url.Values {
	values := url.Values{}
	if includePage {
		page := query.Page
		if page <= 0 {
			page = 1
		}
		pageSize := query.PageSize
		if pageSize <= 0 {
			pageSize = 20
		}
		values.Set("p", strconv.Itoa(page))
		values.Set("page_size", strconv.Itoa(pageSize))
	}
	if query.Type > 0 {
		values.Set("type", strconv.Itoa(query.Type))
	}
	if value := strings.TrimSpace(query.ModelName); value != "" {
		values.Set("model_name", value)
	}
	if query.StartTimestamp > 0 {
		values.Set("start_timestamp", strconv.FormatInt(query.StartTimestamp, 10))
	}
	if query.EndTimestamp > 0 {
		values.Set("end_timestamp", strconv.FormatInt(query.EndTimestamp, 10))
	}
	if query.ChannelID > 0 {
		values.Set("channel", strconv.FormatInt(query.ChannelID, 10))
	}
	if value := strings.TrimSpace(query.RequestID); value != "" {
		values.Set("request_id", value)
	}
	if value := strings.TrimSpace(query.UpstreamRequestID); value != "" {
		values.Set("upstream_request_id", value)
	}
	return values
}

func (c *ReverseClient) DisableChannel(ctx context.Context, channelID int64) error {
	if channelID <= 0 {
		return fmt.Errorf("new-api reverse channel id is required")
	}
	payload := ChannelPayload{
		ID:     channelID,
		Status: ChannelStatusManuallyDisabled,
	}
	var envelope apiEnvelope
	if err := c.doAuthenticatedJSON(ctx, http.MethodPut, "/api/channel/", payload, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("new-api reverse disable channel failed: %s", envelope.Message)
	}
	return nil
}

func (c *ReverseClient) ensureLogin(ctx context.Context) (reverseLoginData, uint64, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if c.authData.ID > 0 {
		return c.authData, c.authVersion, nil
	}
	data, err := c.loginLocked(ctx)
	if err != nil {
		return reverseLoginData{}, c.authVersion, err
	}
	c.authData = data
	c.authVersion++
	return c.authData, c.authVersion, nil
}

func (c *ReverseClient) refreshLogin(ctx context.Context, observedVersion uint64) (reverseLoginData, uint64, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if c.authData.ID > 0 && c.authVersion != observedVersion {
		return c.authData, c.authVersion, nil
	}
	c.authData = reverseLoginData{}
	data, err := c.loginLocked(ctx)
	if err != nil {
		return reverseLoginData{}, c.authVersion, err
	}
	c.authData = data
	c.authVersion++
	return c.authData, c.authVersion, nil
}

func (c *ReverseClient) loginLocked(ctx context.Context) (reverseLoginData, error) {
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/user/login", map[string]string{
		"username": c.username,
		"password": c.password,
	}, 0, "", "", &envelope); err != nil {
		return reverseLoginData{}, err
	}
	if !envelope.Success {
		return reverseLoginData{}, fmt.Errorf("new-api 登录失败: %s", envelope.Message)
	}
	var data reverseLoginData
	if len(bytes.TrimSpace(envelope.Data)) > 0 && string(bytes.TrimSpace(envelope.Data)) != "null" {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return reverseLoginData{}, fmt.Errorf("decode new-api login data: %w", err)
		}
	}
	if data.User != nil {
		if data.ID <= 0 {
			data.ID = data.User.ID
		}
		if data.Role == 0 {
			data.Role = data.User.Role
		}
		data.Require2FA = data.Require2FA || data.User.Require2FA
		data.RequireRootEmailVerification = data.RequireRootEmailVerification || data.User.RequireRootEmailVerification
	}
	if data.Require2FA {
		return reverseLoginData{}, fmt.Errorf("new-api 登录需要二次验证，暂不支持自动校验")
	}
	if data.RequireRootEmailVerification {
		return reverseLoginData{}, fmt.Errorf("new-api 管理员登录需要邮箱验证码，暂不支持自动校验")
	}
	if data.ID <= 0 {
		return reverseLoginData{}, fmt.Errorf("new-api 登录响应缺少用户 ID")
	}
	return data, nil
}

func (c *ReverseClient) doAuthenticatedJSON(ctx context.Context, method string, path string, body any, out any) error {
	login, version, err := c.ensureLogin(ctx)
	if err != nil {
		return err
	}
	err = c.doJSON(ctx, method, path, body, login.ID, login.AccessToken, login.sessionID(), out)
	if !isReverseUnauthorized(err) {
		return err
	}
	login, _, err = c.refreshLogin(ctx, version)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, method, path, body, login.ID, login.AccessToken, login.sessionID(), out)
}

func isReverseUnauthorized(err error) bool {
	var statusErr reverseHTTPStatusError
	return errors.As(err, &statusErr) && statusErr.statusCode == http.StatusUnauthorized
}

func (c *ReverseClient) checkChannelList(ctx context.Context) (int64, error) {
	values := url.Values{}
	values.Set("p", "1")
	values.Set("page_size", "1")
	values.Set("id_sort", "true")

	var envelope apiEnvelope
	if err := c.doAuthenticatedJSON(ctx, http.MethodGet, "/api/channel/?"+values.Encode(), nil, &envelope); err != nil {
		return 0, err
	}
	if !envelope.Success {
		return 0, fmt.Errorf("new-api 渠道列表权限校验失败: %s", envelope.Message)
	}
	var data channelListData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return 0, fmt.Errorf("decode new-api channel list data: %w", err)
	}
	return data.Total, nil
}

func (c *ReverseClient) listChannels(ctx context.Context) ([]Channel, error) {
	const pageSize = 500
	channels := make([]Channel, 0)
	for page := 1; ; page++ {
		values := url.Values{}
		values.Set("p", strconv.Itoa(page))
		values.Set("page_size", strconv.Itoa(pageSize))
		values.Set("id_sort", "true")

		var envelope apiEnvelope
		if err := c.doAuthenticatedJSON(ctx, http.MethodGet, "/api/channel/?"+values.Encode(), nil, &envelope); err != nil {
			return nil, err
		}
		if !envelope.Success {
			return nil, fmt.Errorf("new-api reverse list channels failed: %s", envelope.Message)
		}
		var data channelListData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, fmt.Errorf("decode new-api reverse channel list data: %w", err)
		}
		channels = append(channels, data.Items...)
		if len(data.Items) == 0 || int64(len(channels)) >= data.Total || len(data.Items) < pageSize {
			break
		}
	}
	return channels, nil
}

func (c *ReverseClient) searchChannelByName(ctx context.Context, name string) (*Channel, error) {
	values := url.Values{}
	values.Set("keyword", name)
	values.Set("p", "1")
	values.Set("page_size", "20")
	values.Set("id_sort", "true")

	var envelope apiEnvelope
	if err := c.doAuthenticatedJSON(ctx, http.MethodGet, "/api/channel/search?"+values.Encode(), nil, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("new-api reverse search channel failed: %s", envelope.Message)
	}
	var data searchData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return nil, fmt.Errorf("decode new-api reverse search data: %w", err)
	}
	var matched *Channel
	for index := range data.Items {
		item := &data.Items[index]
		if item.Name == name && (matched == nil || item.ID > matched.ID) {
			matched = item
		}
	}
	if matched != nil {
		return matched, nil
	}
	return nil, fmt.Errorf("new-api reverse channel not found after create: %s", name)
}

// FindUniqueChannelByName is used only with the unique names of API import jobs.
// Ambiguous matches must never be attached to a local key automatically.
func (c *ReverseClient) FindUniqueChannelByName(ctx context.Context, name string) (*Channel, error) {
	channels, err := c.listChannels(ctx)
	if err != nil {
		return nil, err
	}
	var matched *Channel
	for i := range channels {
		if channels[i].Name != name {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("multiple channels match import marker")
		}
		matched = &channels[i]
	}
	return matched, nil
}

func decodeReverseCreateChannelID(data json.RawMessage) (int64, bool, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return 0, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var number json.Number
	if err := decoder.Decode(&number); err == nil {
		id, err := number.Int64()
		if err != nil {
			return 0, false, fmt.Errorf("decode new-api reverse create channel id: %w", err)
		}
		return id, true, nil
	}

	var decoded reverseChannelIDData
	if err := json.Unmarshal(data, &decoded); err != nil {
		return 0, false, fmt.Errorf("decode new-api reverse create channel data: %w", err)
	}
	id, ok := firstReverseChannelID(decoded)
	return id, ok, nil
}

func firstReverseChannelID(data reverseChannelIDData) (int64, bool) {
	if data.ID > 0 {
		return data.ID, true
	}
	if data.ChannelID > 0 {
		return data.ChannelID, true
	}
	if data.ChannelIDCamel > 0 {
		return data.ChannelIDCamel, true
	}
	if data.Channel != nil {
		if id, ok := firstReverseChannelID(*data.Channel); ok {
			return id, true
		}
	}
	if data.CreatedChannel != nil {
		if id, ok := firstReverseChannelID(*data.CreatedChannel); ok {
			return id, true
		}
	}
	for _, channel := range data.CreatedChannels {
		if id, ok := firstReverseChannelID(channel); ok {
			return id, true
		}
	}
	return 0, false
}

func (c *ReverseClient) doJSON(ctx context.Context, method string, path string, body any, userID int, accessToken string, sessionID string, out any) error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal new-api reverse request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create new-api reverse request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if userID > 0 {
		req.Header.Set("New-Api-User", strconv.Itoa(userID))
	}
	if accessToken = strings.TrimSpace(accessToken); accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		req.Header.Set("X-Auth-Session", sessionID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call new-api reverse endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return reverseHTTPStatusError{statusCode: resp.StatusCode}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode new-api reverse response: %w", err)
	}
	return nil
}
