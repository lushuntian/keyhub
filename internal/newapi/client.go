package newapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ChannelStatusEnabled          = 1
	ChannelStatusManuallyDisabled = 2
)

type Client struct {
	baseURL string
	token   string
	userID  int
	client  *http.Client
}

type PushClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type ChannelPayload struct {
	ID            int64   `json:"id,omitempty"`
	Type          int     `json:"type,omitempty"`
	Key           string  `json:"key,omitempty"`
	Status        int     `json:"status,omitempty"`
	Name          string  `json:"name,omitempty"`
	Models        string  `json:"models,omitempty"`
	Group         string  `json:"group,omitempty"`
	BaseURL       *string `json:"base_url,omitempty"`
	Other         string  `json:"other,omitempty"`
	Tag           *string `json:"tag,omitempty"`
	Setting       *string `json:"setting,omitempty"`
	OtherSettings string  `json:"settings,omitempty"`
	Remark        *string `json:"remark,omitempty"`
	AutoBan       *int    `json:"auto_ban,omitempty"`
	Priority      *int64  `json:"priority,omitempty"`
	Weight        *uint   `json:"weight,omitempty"`
}

type AddChannelRequest struct {
	Mode    string          `json:"mode"`
	Channel *ChannelPayload `json:"channel"`
}

type PushChannelRequest struct {
	Source     string         `json:"source"`
	ExternalID string         `json:"external_id"`
	Operation  string         `json:"operation"`
	Channel    ChannelPayload `json:"channel"`
}

type PushChannelResponse struct {
	ChannelID  int64  `json:"channel_id"`
	Action     string `json:"action"`
	ExternalID string `json:"external_id"`
}

type ChannelUsage struct {
	ChannelID    int64 `json:"channel_id"`
	UsedQuota    int64 `json:"used_quota"`
	RequestCount int64 `json:"request_count,omitempty"`
}

type Channel struct {
	ID        int64  `json:"id"`
	Type      int    `json:"type"`
	Status    int    `json:"status"`
	Name      string `json:"name"`
	Tag       string `json:"tag"`
	UsedQuota int64  `json:"used_quota"`
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type searchData struct {
	Items []Channel `json:"items"`
	Total int64     `json:"total"`
}

type channelListData struct {
	Items []Channel `json:"items"`
	Total int64     `json:"total"`
	Page  int       `json:"page"`
}

type channelUsageData struct {
	Items []ChannelUsage `json:"items"`
	Total int64          `json:"total,omitempty"`
}

type TestResult struct {
	Success   bool    `json:"success"`
	Message   string  `json:"message"`
	Time      float64 `json:"time"`
	ErrorCode string  `json:"error_code,omitempty"`
}

func NewClient(baseURL string, token string, userID int) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("new-api base url is required")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("KEYHUB_NEWAPI_ADMIN_TOKEN is required")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("KEYHUB_NEWAPI_ADMIN_USER_ID is required")
	}

	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		userID:  userID,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func NewPushClient(baseURL string, token string) (*PushClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("aggregation target base url is required")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("aggregation target token is required")
	}

	return &PushClient{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (c *PushClient) PushChannel(ctx context.Context, source string, externalID string, payload ChannelPayload) (PushChannelResponse, error) {
	request := PushChannelRequest{
		Source:     source,
		ExternalID: externalID,
		Operation:  "upsert",
		Channel:    payload,
	}
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/keyhub/channels", request, &envelope); err != nil {
		return PushChannelResponse{}, err
	}
	if !envelope.Success {
		return PushChannelResponse{}, fmt.Errorf("new-api keyhub push failed: %s", envelope.Message)
	}
	var response PushChannelResponse
	if err := json.Unmarshal(envelope.Data, &response); err != nil {
		return PushChannelResponse{}, fmt.Errorf("decode new-api keyhub push data: %w", err)
	}
	return response, nil
}

func (c *PushClient) ListChannelUsage(ctx context.Context) ([]ChannelUsage, error) {
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodGet, "/api/keyhub/channels/usage", nil, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("new-api keyhub usage failed: %s", envelope.Message)
	}
	items, err := decodeChannelUsageData(envelope.Data)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ChannelID <= 0 {
			return nil, fmt.Errorf("new-api keyhub usage returned invalid channel_id")
		}
		if item.UsedQuota < 0 {
			return nil, fmt.Errorf("new-api keyhub usage returned negative used_quota for channel %d", item.ChannelID)
		}
	}
	return items, nil
}

func decodeChannelUsageData(data json.RawMessage) ([]ChannelUsage, error) {
	if len(bytes.TrimSpace(data)) == 0 || string(bytes.TrimSpace(data)) == "null" {
		return nil, fmt.Errorf("decode new-api keyhub usage data: data is required")
	}
	var wrapped channelUsageData
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Items != nil {
		return wrapped.Items, nil
	}
	var items []ChannelUsage
	if err := json.Unmarshal(data, &items); err == nil {
		return items, nil
	}
	return nil, fmt.Errorf("decode new-api keyhub usage data: expected data.items or data array")
}

func (c *Client) CreateChannel(ctx context.Context, payload ChannelPayload) error {
	request := AddChannelRequest{
		Mode:    "single",
		Channel: &payload,
	}
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/channel/", request, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("new-api create channel failed: %s", envelope.Message)
	}
	return nil
}

func (c *Client) DisableChannel(ctx context.Context, channelID int64) error {
	payload := ChannelPayload{
		ID:     channelID,
		Status: ChannelStatusManuallyDisabled,
	}
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodPut, "/api/channel/", payload, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("new-api disable channel failed: %s", envelope.Message)
	}
	return nil
}

func (c *Client) SearchChannelByName(ctx context.Context, name string) (*Channel, error) {
	values := url.Values{}
	values.Set("keyword", name)
	values.Set("p", "1")
	values.Set("page_size", "20")
	values.Set("id_sort", "true")
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodGet, "/api/channel/search?"+values.Encode(), nil, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("new-api search channel failed: %s", envelope.Message)
	}
	var data searchData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return nil, fmt.Errorf("decode new-api search data: %w", err)
	}
	for _, item := range data.Items {
		if item.Name == name {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("new-api channel not found after create: %s", name)
}

func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	const pageSize = 500
	channels := make([]Channel, 0)
	for page := 1; ; page++ {
		values := url.Values{}
		values.Set("p", strconv.Itoa(page))
		values.Set("page_size", strconv.Itoa(pageSize))
		values.Set("id_sort", "true")

		var envelope apiEnvelope
		if err := c.doJSON(ctx, http.MethodGet, "/api/channel/?"+values.Encode(), nil, &envelope); err != nil {
			return nil, err
		}
		if !envelope.Success {
			return nil, fmt.Errorf("new-api list channels failed: %s", envelope.Message)
		}
		var data channelListData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, fmt.Errorf("decode new-api channel list data: %w", err)
		}
		channels = append(channels, data.Items...)
		if len(data.Items) == 0 || int64(len(channels)) >= data.Total || len(data.Items) < pageSize {
			break
		}
	}
	return channels, nil
}

func (c *Client) CheckAdmin(ctx context.Context) error {
	values := url.Values{}
	values.Set("p", "1")
	values.Set("page_size", "1")
	values.Set("id_sort", "true")
	var envelope apiEnvelope
	if err := c.doJSON(ctx, http.MethodGet, "/api/channel/?"+values.Encode(), nil, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("new-api admin check failed: %s", envelope.Message)
	}
	return nil
}

func (c *Client) TestChannel(ctx context.Context, channelID int64) (TestResult, error) {
	var result TestResult
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/channel/test/%d", channelID), nil, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *Client) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal new-api request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create new-api request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.token)
	req.Header.Set("New-Api-User", strconv.Itoa(c.userID))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call new-api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("new-api http status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode new-api response: %w", err)
	}
	return nil
}

func (c *PushClient) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal new-api push request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create new-api push request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call new-api push endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("new-api push http status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode new-api push response: %w", err)
	}
	return nil
}
