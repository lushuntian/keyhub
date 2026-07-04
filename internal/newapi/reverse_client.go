package newapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ReverseAdminCheckResult struct {
	UserID       int   `json:"userId"`
	Role         int   `json:"role"`
	ChannelTotal int64 `json:"channelTotal"`
}

type ReverseClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type reverseLoginData struct {
	ID                             int   `json:"id"`
	Role                           int   `json:"role"`
	Require2FA                     bool  `json:"require_2fa"`
	RequireRootEmailVerification   bool  `json:"require_root_email_verification"`
	RootLoginVerificationEmailList []any `json:"root_login_verification_emails"`
}

type reverseChannelIDData struct {
	ID              int64                  `json:"id"`
	ChannelID       int64                  `json:"channel_id"`
	ChannelIDCamel  int64                  `json:"channelId"`
	Channel         *reverseChannelIDData  `json:"channel"`
	CreatedChannel  *reverseChannelIDData  `json:"createdChannel"`
	CreatedChannels []reverseChannelIDData `json:"createdChannels"`
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
	login, err := c.login(ctx)
	if err != nil {
		return ReverseAdminCheckResult{}, err
	}
	total, err := c.checkChannelList(ctx, login.ID)
	if err != nil {
		return ReverseAdminCheckResult{}, err
	}
	return ReverseAdminCheckResult{
		UserID:       login.ID,
		Role:         login.Role,
		ChannelTotal: total,
	}, nil
}

func (c *ReverseClient) CreateChannel(ctx context.Context, payload ChannelPayload) (PushChannelResponse, error) {
	login, err := c.login(ctx)
	if err != nil {
		return PushChannelResponse{}, err
	}
	request := AddChannelRequest{
		Mode:    "single",
		Channel: &payload,
	}
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/channel/", request, login.ID, &envelope); err != nil {
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
		channel, err := c.searchChannelByName(ctx, login.ID, payload.Name)
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
	login, err := c.login(ctx)
	if err != nil {
		return nil, err
	}
	channels, err := c.listChannels(ctx, login.ID)
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
		})
	}
	return items, nil
}

func (c *ReverseClient) DisableChannel(ctx context.Context, channelID int64) error {
	if channelID <= 0 {
		return fmt.Errorf("new-api reverse channel id is required")
	}
	login, err := c.login(ctx)
	if err != nil {
		return err
	}
	payload := ChannelPayload{
		ID:     channelID,
		Status: ChannelStatusManuallyDisabled,
	}
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPut, "/api/channel/", payload, login.ID, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("new-api reverse disable channel failed: %s", envelope.Message)
	}
	return nil
}

func (c *ReverseClient) login(ctx context.Context) (reverseLoginData, error) {
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/user/login", map[string]string{
		"username": c.username,
		"password": c.password,
	}, 0, &envelope); err != nil {
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

func (c *ReverseClient) checkChannelList(ctx context.Context, userID int) (int64, error) {
	values := url.Values{}
	values.Set("p", "1")
	values.Set("page_size", "1")
	values.Set("id_sort", "true")

	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodGet, "/api/channel/?"+values.Encode(), nil, userID, &envelope); err != nil {
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

func (c *ReverseClient) listChannels(ctx context.Context, userID int) ([]Channel, error) {
	const pageSize = 500
	channels := make([]Channel, 0)
	for page := 1; ; page++ {
		values := url.Values{}
		values.Set("p", strconv.Itoa(page))
		values.Set("page_size", strconv.Itoa(pageSize))
		values.Set("id_sort", "true")

		var envelope apiEnvelope
		if err := c.doJSON(ctx, http.MethodGet, "/api/channel/?"+values.Encode(), nil, userID, &envelope); err != nil {
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

func (c *ReverseClient) searchChannelByName(ctx context.Context, userID int, name string) (*Channel, error) {
	values := url.Values{}
	values.Set("keyword", name)
	values.Set("p", "1")
	values.Set("page_size", "20")
	values.Set("id_sort", "true")

	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodGet, "/api/channel/search?"+values.Encode(), nil, userID, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("new-api reverse search channel failed: %s", envelope.Message)
	}
	var data searchData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return nil, fmt.Errorf("decode new-api reverse search data: %w", err)
	}
	for _, item := range data.Items {
		if item.Name == name {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("new-api reverse channel not found after create: %s", name)
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

func (c *ReverseClient) doJSON(ctx context.Context, method string, path string, body any, userID int, out any) error {
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
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call new-api reverse endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("new-api reverse http status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode new-api reverse response: %w", err)
	}
	return nil
}
