package httpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"keyhub/internal/database"
	"keyhub/internal/newapi"
)

func TestSanitizeUsageLogOnlyReturnsWhitelistedDetails(t *testing.T) {
	item := sanitizeUsageLog(newapi.UsageLog{
		ID:                91,
		CreatedAt:         123,
		Type:              2,
		Content:           "ok",
		ModelName:         "claude-opus-4-8",
		Quota:             500000,
		PromptTokens:      10,
		CompletionTokens:  3,
		UseTime:           2,
		IsStream:          true,
		ChannelID:         999,
		RequestID:         "req-1",
		UpstreamRequestID: "upstream-1",
		Other: `{
			"frt": 321,
			"cache_tokens": 7,
			"cache_creation_tokens_5m": 2,
			"cache_creation_tokens_1h": 3,
			"proxy_pool_id": 88,
			"group": "internal",
			"admin_info": {"use_channel": [999]},
			"stream_status": {"status": "finished", "end_reason": "stop", "errors": ["secret"]}
		}`,
	}, 42, 500000)

	if item.LocalChannelID != 42 {
		t.Fatalf("channel id = %d, want 42", item.LocalChannelID)
	}
	if item.QuotaUSD != 1 {
		t.Fatalf("quota USD = %f, want 1", item.QuotaUSD)
	}
	if item.FirstResponseTimeMS != 321 ||
		item.CacheReadTokens != 7 ||
		item.CacheCreationTokens != 5 ||
		item.StreamStatus != "finished" ||
		item.StreamEndReason != "stop" {
		t.Fatalf("sanitized usage log = %+v", item)
	}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal sanitized usage log: %v", err)
	}
	for _, sensitive := range []string{"999", "proxy_pool_id", "internal", "admin_info", "secret"} {
		if strings.Contains(string(payload), sensitive) {
			t.Fatalf("sanitized payload contains %q: %s", sensitive, payload)
		}
	}
}

func TestUsageLogCursorRoundTrip(t *testing.T) {
	encoded, err := encodeUsageLogCursor(usageLogCursor{
		TargetCode: "target-a",
		FilterHash: "hash",
		Consumed:   map[string]int{"44": 3},
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := decodeUsageLogCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if decoded.TargetCode != "target-a" || decoded.FilterHash != "hash" || decoded.Consumed["44"] != 3 {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
	if _, err := decodeUsageLogCursor("not-base64"); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid cursor error = %v", err)
	}
}

func TestFilterUsageLogMappingsUsesLocalChannelID(t *testing.T) {
	mappings := []database.UsageLogChannelMapping{
		{LocalChannelID: 10, RemoteChannelID: 100},
		{LocalChannelID: 11, RemoteChannelID: 101},
	}
	filtered := filterUsageLogMappings(mappings, 11)
	if len(filtered) != 1 || filtered[0].RemoteChannelID != 101 {
		t.Fatalf("filtered mappings = %+v", filtered)
	}
	if got := filterUsageLogMappings(mappings, 99); len(got) != 0 {
		t.Fatalf("unauthorized local channel mappings = %+v", got)
	}
}

func TestRedactUsageLogContent(t *testing.T) {
	input := "status_code=429, channel_id=61, token_name=internal-token, proxy_pool_id=#151"
	output := redactUsageLogContent(input)
	for _, sensitive := range []string{"61", "internal-token", "#151"} {
		if strings.Contains(output, sensitive) {
			t.Fatalf("redacted content contains %q: %s", sensitive, output)
		}
	}
	if !strings.Contains(output, "status_code=429") {
		t.Fatalf("redacted content lost safe status: %s", output)
	}
}
