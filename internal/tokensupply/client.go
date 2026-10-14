package tokensupply

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type CheckResult struct {
	Username         string `json:"username"`
	PurchaseProvider string `json:"purchaseProvider"`
	ProviderCount    int    `json:"providerCount"`
	APIKeyCount      int    `json:"apiKeyCount"`
}

type Provider struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}

type AddAPIKeyRequest struct {
	APIKey        string  `json:"api_key"`
	ProviderSlug  string  `json:"provider_slug"`
	HasQuotaLimit int     `json:"has_quota_limit"`
	QuotaLimit    *string `json:"quota_limit,omitempty"`
	Description   string  `json:"description,omitempty"`
	BizDate       string  `json:"biz_date"`
}

type APIKeyRecord struct {
	ID                   string          `json:"id"`
	ProviderSlug         string          `json:"provider_slug"`
	HasQuotaLimit        int             `json:"has_quota_limit"`
	QuotaLimit           json.RawMessage `json:"quota_limit"`
	ActualCost           json.RawMessage `json:"actual_cost"`
	BizDate              string          `json:"biz_date"`
	Description          string          `json:"description"`
	ProviderCredentialID string          `json:"provider_credential_id"`
	Consumed             int             `json:"consumed"`
}

type Usage struct {
	RemoteID  int64
	UsedQuota int64
}

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type loginData struct {
	Username         string `json:"username"`
	PurchaseProvider string `json:"purchase_provider"`
}

func NewClient(baseURL string, username string, password string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("tokensupply base url is required")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("tokensupply username is required")
	}
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("tokensupply password is required")
	}
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: strings.TrimSpace(password),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (c *Client) CheckAccount(ctx context.Context) (CheckResult, error) {
	login, err := c.login(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	providers, err := c.listProviders(ctx, false)
	if err != nil {
		return CheckResult{}, err
	}
	keys, err := c.listAPIKeys(ctx, false)
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{
		Username:         login.Username,
		PurchaseProvider: login.PurchaseProvider,
		ProviderCount:    len(providers),
		APIKeyCount:      len(keys),
	}, nil
}

func (c *Client) AddAPIKey(ctx context.Context, request AddAPIKeyRequest) (APIKeyRecord, error) {
	if strings.TrimSpace(request.APIKey) == "" {
		return APIKeyRecord{}, fmt.Errorf("tokensupply api_key is required")
	}
	if strings.TrimSpace(request.ProviderSlug) == "" {
		return APIKeyRecord{}, fmt.Errorf("tokensupply provider_slug is required")
	}
	if strings.TrimSpace(request.BizDate) == "" {
		request.BizDate = time.Now().Format("20060102")
	}
	if _, err := c.login(ctx); err != nil {
		return APIKeyRecord{}, err
	}
	var envelope envelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/apikey/add", request, true, &envelope); err != nil {
		return APIKeyRecord{}, err
	}
	if !envelope.Success {
		return APIKeyRecord{}, fmt.Errorf("tokensupply add api key failed: %s", envelope.Message)
	}
	var record APIKeyRecord
	if err := json.Unmarshal(envelope.Data, &record); err != nil {
		return APIKeyRecord{}, fmt.Errorf("decode tokensupply add api key data: %w", err)
	}
	if strings.TrimSpace(record.ID) == "" {
		return APIKeyRecord{}, fmt.Errorf("tokensupply add api key returned empty id")
	}
	return record, nil
}

func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKeyRecord, error) {
	return c.listAPIKeys(ctx, true)
}

func (c *Client) DeleteAPIKeyByRemoteID(ctx context.Context, remoteID int64) error {
	if remoteID <= 0 {
		return fmt.Errorf("tokensupply remote id is required")
	}
	records, err := c.ListAPIKeys(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if RemoteIDNumber(record.ID) == remoteID {
			return c.DeleteAPIKey(ctx, record.ID)
		}
	}
	return fmt.Errorf("tokensupply api key not found for remote id %d", remoteID)
}

func (c *Client) DeleteAPIKey(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("tokensupply api key id is required")
	}
	if _, err := c.login(ctx); err != nil {
		return err
	}
	var envelope envelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/apikey/delete", map[string]string{"id": id}, true, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("tokensupply delete api key failed: %s", envelope.Message)
	}
	return nil
}

func (c *Client) ListUsage(ctx context.Context, quotaPerUSD float64) ([]Usage, error) {
	if quotaPerUSD <= 0 {
		quotaPerUSD = 500000
	}
	records, err := c.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Usage, 0, len(records))
	for _, record := range records {
		costUSD, err := parseActualCost(record.ActualCost)
		if err != nil {
			return nil, fmt.Errorf("tokensupply api key %s actual_cost: %w", record.ID, err)
		}
		if costUSD < 0 {
			return nil, fmt.Errorf("tokensupply api key %s returned negative actual_cost", record.ID)
		}
		items = append(items, Usage{
			RemoteID:  RemoteIDNumber(record.ID),
			UsedQuota: int64(math.Round(costUSD * quotaPerUSD)),
		})
	}
	return items, nil
}

func (c *Client) listProviders(ctx context.Context, ensureLogin bool) ([]Provider, error) {
	if ensureLogin {
		if _, err := c.login(ctx); err != nil {
			return nil, err
		}
	}
	var envelope envelope
	if err := c.doJSON(ctx, http.MethodGet, "/api/provider/list", nil, true, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("tokensupply provider list failed: %s", envelope.Message)
	}
	var providers []Provider
	if err := json.Unmarshal(envelope.Data, &providers); err != nil {
		return nil, fmt.Errorf("decode tokensupply provider list data: %w", err)
	}
	return providers, nil
}

func (c *Client) listAPIKeys(ctx context.Context, ensureLogin bool) ([]APIKeyRecord, error) {
	if ensureLogin {
		if _, err := c.login(ctx); err != nil {
			return nil, err
		}
	}
	var envelope envelope
	if err := c.doJSON(ctx, http.MethodGet, "/api/apikey/list", nil, true, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("tokensupply api key list failed: %s", envelope.Message)
	}
	var records []APIKeyRecord
	if err := json.Unmarshal(envelope.Data, &records); err != nil {
		return nil, fmt.Errorf("decode tokensupply api key list data: %w", err)
	}
	return records, nil
}

func (c *Client) login(ctx context.Context) (loginData, error) {
	var envelope envelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/user/login", map[string]string{
		"username": c.username,
		"password": c.password,
	}, false, &envelope); err != nil {
		return loginData{}, err
	}
	if !envelope.Success {
		return loginData{}, fmt.Errorf("tokensupply login failed: %s", envelope.Message)
	}
	var data loginData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return loginData{}, fmt.Errorf("decode tokensupply login data: %w", err)
	}
	if strings.TrimSpace(data.Username) == "" {
		return loginData{}, fmt.Errorf("tokensupply login response missing username")
	}
	return data, nil
}

func (c *Client) doJSON(ctx context.Context, method string, path string, body any, providerHeader bool, out *envelope) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal tokensupply request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create tokensupply request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if providerHeader {
		req.Header.Set("x-provider-user", c.username)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call tokensupply endpoint: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read tokensupply response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tokensupply http status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("decode tokensupply response: %w", err)
	}
	return nil
}

func parseActualCost(raw json.RawMessage) (float64, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "$"))
	if text == "" {
		return 0, nil
	}
	return strconv.ParseFloat(text, 64)
}

func RemoteIDNumber(remoteID string) int64 {
	sum := sha256.Sum256([]byte(strings.TrimSpace(remoteID)))
	value := int64(binary.BigEndian.Uint64(sum[:8]) & math.MaxInt64)
	if value == 0 {
		return 1
	}
	return value
}
