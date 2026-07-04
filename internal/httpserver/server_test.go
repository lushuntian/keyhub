package httpserver

import (
	"testing"

	"keyhub/internal/database"
	"keyhub/internal/newapi"
)

func TestBuildNewAPIChannelPayloadDefaultsDisabledAndDefaultGroup(t *testing.T) {
	payload := buildNewAPIChannelPayload(database.APIKeyForSync{
		ID:           12,
		CategoryCode: "openai",
		NewAPIType:   1,
		KeyHint:      "sk-test",
		Models:       []string{"gpt-4o"},
	}, "sk-test")

	if payload.Status != newapi.ChannelStatusManuallyDisabled {
		t.Fatalf("status = %d, want %d", payload.Status, newapi.ChannelStatusManuallyDisabled)
	}
	if payload.Group != "default" {
		t.Fatalf("group = %q, want default", payload.Group)
	}
	if payload.Key != "sk-test" {
		t.Fatalf("key = %q, want sk-test", payload.Key)
	}
}

func TestBuildNewAPIChannelPayloadSplitsAzureKey(t *testing.T) {
	payload := buildNewAPIChannelPayload(database.APIKeyForSync{
		ID:           34,
		CategoryCode: "azure_openai",
		NewAPIType:   3,
		KeyHint:      "https://example.openai.azure.com|secret|2024-12-01-preview",
		Models:       []string{"gpt-4.1"},
	}, "https://example.openai.azure.com/|secret|2024-12-01-preview")

	if payload.Key != "secret" {
		t.Fatalf("key = %q, want secret", payload.Key)
	}
	if payload.BaseURL == nil || *payload.BaseURL != "https://example.openai.azure.com" {
		t.Fatalf("base_url = %v, want https://example.openai.azure.com", payload.BaseURL)
	}
	if payload.Other != "2024-12-01-preview" {
		t.Fatalf("other = %q, want 2024-12-01-preview", payload.Other)
	}
	if payload.Status != newapi.ChannelStatusManuallyDisabled {
		t.Fatalf("status = %d, want %d", payload.Status, newapi.ChannelStatusManuallyDisabled)
	}
	if payload.Group != "default" {
		t.Fatalf("group = %q, want default", payload.Group)
	}
}

func TestValidateAggregationTargetInputAcceptsIPPortURL(t *testing.T) {
	err := validateAggregationTargetInput("default", "default", "http://localhost:3013")
	if err != nil {
		t.Fatalf("validateAggregationTargetInput returned error: %v", err)
	}
}
