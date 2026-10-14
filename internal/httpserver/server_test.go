package httpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/keys"
	"keyhub/internal/newapi"
)

func TestBuildNewAPIChannelPayloadDefaultsEnabledAndDefaultGroup(t *testing.T) {
	payload := buildNewAPIChannelPayload(database.APIKeyForSync{
		ID:           12,
		CategoryCode: "openai",
		NewAPIType:   1,
		KeyHint:      "sk-test",
		BaseURL:      "https://api.openai.com/v1",
		Models:       []string{"gpt-4o"},
	}, "sk-test", "openai", 1, []string{"gpt-4o"})

	if payload.Status != newapi.ChannelStatusEnabled {
		t.Fatalf("status = %d, want %d", payload.Status, newapi.ChannelStatusEnabled)
	}
	if payload.Group != "default" {
		t.Fatalf("group = %q, want default", payload.Group)
	}
	if payload.Key != "sk-test" {
		t.Fatalf("key = %q, want sk-test", payload.Key)
	}
	if payload.Models != "gpt-4o" {
		t.Fatalf("models = %q, want gpt-4o", payload.Models)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(payloadJSON), "base_url") {
		t.Fatalf("payload unexpectedly contains base_url: %s", payloadJSON)
	}
}

func TestBuildNewAPIChannelPayloadUsesConfiguredModels(t *testing.T) {
	payload := buildNewAPIChannelPayload(database.APIKeyForSync{
		ID:           12,
		CategoryCode: "anthropic",
		NewAPIType:   14,
		KeyHint:      "sk-ant-test",
	}, "sk-ant-test", "anthropic", 14, []string{" claude-sonnet-4-6 ", "claude-opus-4-6", "claude-sonnet-4-6"})

	want := "claude-sonnet-4-6,claude-opus-4-6"
	if payload.Models != want {
		t.Fatalf("models = %q, want %q", payload.Models, want)
	}
}

func TestBuildNewAPIChannelPayloadUsesSelectedAWSCredentialType(t *testing.T) {
	tests := []struct {
		name           string
		credentialType string
		plaintextKey   string
		want           string
	}{
		{name: "api key", credentialType: awsCredentialTypeAPIKey, plaintextKey: "api-key|us-east-1", want: awsCredentialTypeAPIKey},
		{name: "ak sk", credentialType: awsCredentialTypeAKSK, plaintextKey: "AK|SK|us-east-1", want: awsCredentialTypeAKSK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildNewAPIChannelPayload(database.APIKeyForSync{
				ID:                24,
				AWSCredentialType: tt.credentialType,
			}, tt.plaintextKey, aggregationTargetChannelTypeAWSBedrockLegacy, 33, []string{"claude-opus-4-8"})

			var settings map[string]string
			if err := json.Unmarshal([]byte(payload.OtherSettings), &settings); err != nil {
				t.Fatalf("unmarshal settings: %v", err)
			}
			if settings["aws_key_type"] != tt.want {
				t.Fatalf("aws_key_type = %q, want %q", settings["aws_key_type"], tt.want)
			}
		})
	}
}

func TestAWSBedrockChannelVersionsUseDifferentPayloadTypes(t *testing.T) {
	keyRecord := database.APIKeyForSync{
		ID:                25,
		CreatedBy:         "admin",
		AWSCredentialType: awsCredentialTypeAKSK,
	}

	legacy := buildNewAPIChannelPayload(
		keyRecord,
		"AK|SK|us-east-1",
		aggregationTargetChannelTypeAWSBedrockLegacy,
		33,
		[]string{"claude-sonnet-4-6"},
	)
	if legacy.Type != 33 {
		t.Fatalf("legacy payload type = %d, want 33", legacy.Type)
	}
	if legacy.Name != "admin_aws_bedrock" {
		t.Fatalf("legacy payload name = %q, want admin_aws_bedrock", legacy.Name)
	}
	if !strings.Contains(legacy.OtherSettings, "aws_key_type") {
		t.Fatalf("legacy other settings = %q, want aws_key_type", legacy.OtherSettings)
	}
	if legacy.ModelMapping != "" {
		t.Fatalf("legacy model mapping = %q, want empty", legacy.ModelMapping)
	}

	modernModels := []string{"claude-sonnet-5", "claude-opus-4-7", "claude-opus-4-8"}
	modern := buildNewAPIChannelPayload(
		keyRecord,
		"sk-ant-test",
		aggregationTargetChannelTypeAWSBedrockNew,
		14,
		modernModels,
	)
	if modern.Type != 14 {
		t.Fatalf("modern payload type = %d, want Anthropic type 14", modern.Type)
	}
	if modern.Name != "admin_aws_bedrock_new" {
		t.Fatalf("modern payload name = %q, want admin_aws_bedrock_new", modern.Name)
	}
	if modern.Key != "sk-ant-test" || modern.OtherSettings != "{}" {
		t.Fatalf("modern payload = %#v, want Anthropic content without AWS settings", modern)
	}
	var modelMapping map[string]string
	if err := json.Unmarshal([]byte(modern.ModelMapping), &modelMapping); err != nil {
		t.Fatalf("unmarshal modern model mapping: %v", err)
	}
	for _, model := range modernModels {
		if got, want := modelMapping[model], "anthropic."+model; got != want {
			t.Fatalf("model mapping[%q] = %q, want %q", model, got, want)
		}
	}
	modernJSON, err := json.Marshal(modern)
	if err != nil {
		t.Fatalf("marshal modern payload: %v", err)
	}
	var wirePayload map[string]any
	if err := json.Unmarshal(modernJSON, &wirePayload); err != nil {
		t.Fatalf("unmarshal modern wire payload: %v", err)
	}
	if got, ok := wirePayload["model_mapping"].(string); !ok || got != modern.ModelMapping {
		t.Fatalf("wire model_mapping = %#v, want JSON string %q", wirePayload["model_mapping"], modern.ModelMapping)
	}
}

func TestBuildAWSBedrockNewModelMappingOnlyAppliesToModernBedrock(t *testing.T) {
	models := []string{" claude-sonnet-5 ", "claude-sonnet-5", "claude-opus-4-8"}
	if got := buildAWSBedrockNewModelMapping("anthropic", models); got != "" {
		t.Fatalf("Anthropic model mapping = %q, want empty", got)
	}
	if got := buildAWSBedrockNewModelMapping(aggregationTargetChannelTypeAWSBedrockLegacy, models); got != "" {
		t.Fatalf("legacy Bedrock model mapping = %q, want empty", got)
	}

	got := buildAWSBedrockNewModelMapping(aggregationTargetChannelTypeAWSBedrockNew, models)
	var mapping map[string]string
	if err := json.Unmarshal([]byte(got), &mapping); err != nil {
		t.Fatalf("unmarshal model mapping: %v", err)
	}
	if len(mapping) != 2 {
		t.Fatalf("mapping count = %d, want 2 unique models", len(mapping))
	}
	if mapping["claude-sonnet-5"] != "anthropic.claude-sonnet-5" {
		t.Fatalf("sonnet mapping = %q", mapping["claude-sonnet-5"])
	}
}

func TestNormalizeAWSBedrockNewRegions(t *testing.T) {
	regions, err := normalizeAWSBedrockNewRegions([]string{
		" https://bedrock-mantle.eu-north-1.api.aws/anthropic ",
		"",
		"https://bedrock-mantle.eu-north-1.api.aws/anthropic",
		"https://bedrock-mantle.us-east-1.api.aws/anthropic",
	})
	if err != nil {
		t.Fatalf("normalizeAWSBedrockNewRegions() error = %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("regions count = %d, want 2 unique regions", len(regions))
	}
	if regions[0] != "https://bedrock-mantle.eu-north-1.api.aws/anthropic" {
		t.Fatalf("first region = %q", regions[0])
	}

	if _, err := normalizeAWSBedrockNewRegions(nil); err == nil {
		t.Fatal("empty regions error = nil, want required error")
	}
	if _, err := normalizeAWSBedrockNewRegions([]string{"https://example.com/anthropic"}); err == nil {
		t.Fatal("invalid prefix error = nil, want prefix error")
	}
}

func TestExpandParsedKeysByBaseURLsBuildsCartesianProduct(t *testing.T) {
	items := keys.ParseLines("sk-first\nsk-second\nsk-third")
	regions := []string{
		"https://bedrock-mantle.eu-north-1.api.aws/anthropic",
		"https://bedrock-mantle.us-east-1.api.aws/anthropic",
	}

	expanded := expandParsedKeysByBaseURLs(items, regions)
	if len(expanded) != 6 {
		t.Fatalf("expanded count = %d, want 6", len(expanded))
	}
	fingerprints := make(map[string]struct{}, len(expanded))
	for _, item := range expanded {
		if item.BaseURL == "" {
			t.Fatal("expanded item base URL is empty")
		}
		fingerprints[item.Fingerprint] = struct{}{}
	}
	if len(fingerprints) != 6 {
		t.Fatalf("unique fingerprint count = %d, want 6", len(fingerprints))
	}
}

func TestApplyAWSBedrockNewRegionSetsBaseURLOnlyForReverseModernBedrock(t *testing.T) {
	keyRecord := database.APIKeyForSync{
		BaseURL: "https://bedrock-mantle.eu-north-1.api.aws/anthropic",
	}
	reverseTarget := config.AggregationTarget{ConnectionMode: aggregationTargetConnectionModeNewAPIReverse}

	modern := applyAWSBedrockNewRegion(
		reverseTarget,
		keyRecord,
		aggregationTargetChannelTypeAWSBedrockNew,
		newapi.ChannelPayload{},
	)
	if modern.BaseURL != keyRecord.BaseURL {
		t.Fatalf("modern reverse base URL = %q, want %q", modern.BaseURL, keyRecord.BaseURL)
	}
	modernJSON, err := json.Marshal(modern)
	if err != nil {
		t.Fatalf("marshal modern reverse payload: %v", err)
	}
	if !strings.Contains(string(modernJSON), `"base_url":"https://bedrock-mantle.eu-north-1.api.aws/anthropic"`) {
		t.Fatalf("modern reverse payload missing base_url: %s", modernJSON)
	}

	legacy := applyAWSBedrockNewRegion(
		reverseTarget,
		keyRecord,
		aggregationTargetChannelTypeAWSBedrockLegacy,
		newapi.ChannelPayload{},
	)
	if legacy.BaseURL != "" {
		t.Fatalf("legacy reverse base URL = %q, want empty", legacy.BaseURL)
	}

	apiMode := applyAWSBedrockNewRegion(
		config.AggregationTarget{ConnectionMode: aggregationTargetConnectionModeAPI},
		keyRecord,
		aggregationTargetChannelTypeAWSBedrockNew,
		newapi.ChannelPayload{},
	)
	if apiMode.BaseURL != "" {
		t.Fatalf("API mode base URL = %q, want empty", apiMode.BaseURL)
	}
}

func TestAggregationTargetPayloadCategoryCodeMapsNewAWSBedrockToAnthropic(t *testing.T) {
	if got := aggregationTargetPayloadCategoryCode(aggregationTargetChannelTypeAWSBedrockLegacy); got != aggregationTargetChannelTypeAWSBedrockLegacy {
		t.Fatalf("legacy payload category = %q, want %q", got, aggregationTargetChannelTypeAWSBedrockLegacy)
	}
	if got := aggregationTargetPayloadCategoryCode(aggregationTargetChannelTypeAWSBedrockNew); got != "anthropic" {
		t.Fatalf("modern payload category = %q, want anthropic", got)
	}
}

func TestValidateAWSCredentialLines(t *testing.T) {
	tests := []struct {
		name           string
		credentialType string
		rawText        string
		wantError      bool
	}{
		{name: "valid api key", credentialType: awsCredentialTypeAPIKey, rawText: "api-key|us-east-1"},
		{name: "api key rejects ak sk", credentialType: awsCredentialTypeAPIKey, rawText: "AK|SK|us-east-1", wantError: true},
		{name: "valid ak sk", credentialType: awsCredentialTypeAKSK, rawText: "AK|SK|us-east-1"},
		{name: "ak sk rejects empty part", credentialType: awsCredentialTypeAKSK, rawText: "AK||us-east-1", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAWSCredentialLines(keys.ParseLines(tt.rawText), tt.credentialType)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateAWSCredentialLines() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestFilterAggregationTargetsByChannelTypes(t *testing.T) {
	targets := []config.AggregationTarget{
		{Code: "anthropic-a", ChannelType: "anthropic"},
		{Code: "anthropic-b", ChannelType: "anthropic"},
		{Code: "openai-a", ChannelType: "openai"},
	}

	filtered, err := filterAggregationTargetsByChannelTypes(targets, []string{"anthropic", "anthropic"})
	if err != nil {
		t.Fatalf("filterAggregationTargetsByChannelTypes() error = %v", err)
	}
	if len(filtered) != 2 || filtered[0].Code != "anthropic-a" || filtered[1].Code != "anthropic-b" {
		t.Fatalf("filtered targets = %#v, want both anthropic targets", filtered)
	}

	all, err := filterAggregationTargetsByChannelTypes(targets, nil)
	if err != nil {
		t.Fatalf("filter all targets: %v", err)
	}
	if len(all) != len(targets) {
		t.Fatalf("all targets count = %d, want %d", len(all), len(targets))
	}
}

func TestFilterAggregationTargetsByChannelTypesRejectsUnauthorizedType(t *testing.T) {
	targets := []config.AggregationTarget{{Code: "anthropic-a", ChannelType: "anthropic"}}

	if _, err := filterAggregationTargetsByChannelTypes(targets, []string{"openai"}); err == nil {
		t.Fatal("filterAggregationTargetsByChannelTypes() error = nil, want unauthorized channel type error")
	}
	if _, err := filterAggregationTargetsByChannelTypes(targets, []string{"not-a-channel"}); err == nil {
		t.Fatal("filterAggregationTargetsByChannelTypes() error = nil, want unsupported channel type error")
	}
}

func TestRegularUserAggregationTargetResponsesGroupsAndHidesPlatforms(t *testing.T) {
	records := []database.AggregationTargetRecord{
		{Code: "anthropic-a", Name: "生产平台", BaseURL: "https://a.example.com", ChannelType: "anthropic", Models: []string{"claude-sonnet-4-6"}},
		{Code: "anthropic-b", Name: "备用平台", BaseURL: "https://b.example.com", ChannelType: "anthropic", Models: []string{"claude-opus-4-6"}},
		{Code: "openai-a", Name: "其他平台", BaseURL: "https://c.example.com", ChannelType: "openai", Models: []string{"gpt-5.2"}},
	}

	items := regularUserAggregationTargetResponses(records)
	if len(items) != 2 {
		t.Fatalf("items count = %d, want 2 channel types", len(items))
	}
	if items[0].ChannelType != "anthropic" || items[0].TargetCount != 2 {
		t.Fatalf("first item = %#v, want anthropic count 2", items[0])
	}
	if items[1].ChannelType != "openai" || items[1].TargetCount != 1 {
		t.Fatalf("second item = %#v, want openai count 1", items[1])
	}
	if len(items[0].Models) != 1 || items[0].Models[0] != "claude-sonnet-4-6" {
		t.Fatalf("anthropic sample models = %q, want first platform models", items[0].Models)
	}
	if len(items[1].Models) != 1 || items[1].Models[0] != "gpt-5.2" {
		t.Fatalf("openai sample models = %q, want first platform models", items[1].Models)
	}
	for _, item := range items {
		if item.Code != "" || item.Name != "" || item.BaseURL != "" {
			t.Fatalf("regular user item leaks platform details: %#v", item)
		}
	}
}

func TestChannelModelsForTargetUsesKeyOverride(t *testing.T) {
	got := channelModelsForTarget(
		[]string{" custom-model ", "custom-model", "another-model"},
		[]string{"target-model"},
	)
	if strings.Join(got, ",") != "custom-model,another-model" {
		t.Fatalf("override models = %q, want cleaned key models", got)
	}
}

func TestChannelModelsForTargetFallsBackToTarget(t *testing.T) {
	got := channelModelsForTarget(nil, []string{" target-model ", "target-model"})
	if len(got) != 1 || got[0] != "target-model" {
		t.Fatalf("fallback models = %q, want cleaned target models", got)
	}
}

func TestChannelModelsForPayloadDoesNotUseGlobalFallback(t *testing.T) {
	if models := channelModelsForPayload(nil); len(models) != 0 {
		t.Fatalf("models = %q, want empty", models)
	}
}

func TestBuildChannelNameUsesKeyOwnerAndCategory(t *testing.T) {
	got := buildChannelName(database.APIKeyForSync{
		CreatedBy:    "admin",
		CategoryCode: "anthropic",
	}, "anthropic")
	if got != "admin_anthropic" {
		t.Fatalf("buildChannelName() = %q, want admin_anthropic", got)
	}
}

func TestBuildChannelNameFallsBackForHistoricalOwner(t *testing.T) {
	got := buildChannelName(database.APIKeyForSync{}, "openai")
	if got != "keyhub_openai" {
		t.Fatalf("buildChannelName() = %q, want keyhub_openai", got)
	}
}

func TestApplyAggregationTargetChannelSettingsUsesReverseGroup(t *testing.T) {
	payload := applyAggregationTargetChannelSettings(config.AggregationTarget{
		ConnectionMode:   aggregationTargetConnectionModeNewAPIReverse,
		ReverseGroupName: "anthropic",
	}, newapi.ChannelPayload{Group: "default"})

	if payload.Group != "anthropic" {
		t.Fatalf("group = %q, want anthropic", payload.Group)
	}

	payload = applyAggregationTargetChannelSettings(config.AggregationTarget{
		ConnectionMode: aggregationTargetConnectionModeNewAPIReverse,
	}, newapi.ChannelPayload{Group: "custom"})
	if payload.Group != "default" {
		t.Fatalf("empty reverse group = %q, want default", payload.Group)
	}
}

func TestBuildNewAPIChannelPayloadKeepsRawKey(t *testing.T) {
	payload := buildNewAPIChannelPayload(database.APIKeyForSync{
		ID:           34,
		CategoryCode: "azure_openai",
		NewAPIType:   3,
		KeyHint:      "https://example.openai.azure.com|secret|2024-12-01-preview",
		BaseURL:      "https://example.openai.azure.com",
		Models:       []string{"gpt-4.1"},
	}, "opaque|credential|value", "azure_openai", 3, []string{"gpt-4.1"})

	if payload.Key != "opaque|credential|value" {
		t.Fatalf("key = %q, want raw credential", payload.Key)
	}
	if payload.Other != "" {
		t.Fatalf("other = %q, want empty", payload.Other)
	}
	if payload.Status != newapi.ChannelStatusEnabled {
		t.Fatalf("status = %d, want %d", payload.Status, newapi.ChannelStatusEnabled)
	}
	if payload.Group != "default" {
		t.Fatalf("group = %q, want default", payload.Group)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(payloadJSON), "base_url") {
		t.Fatalf("Azure payload unexpectedly contains base_url: %s", payloadJSON)
	}
}

func TestValidateAggregationTargetInputAcceptsIPPortURL(t *testing.T) {
	err := validateAggregationTargetInput("default", "default", "http://localhost:3013")
	if err != nil {
		t.Fatalf("validateAggregationTargetInput returned error: %v", err)
	}
}

func TestTokenSupplyProviderSlug(t *testing.T) {
	tests := []struct {
		channelType int
		want        string
	}{
		{channelType: 1, want: "openai"},
		{channelType: 3, want: "azure"},
		{channelType: 14, want: "anthropic"},
		{channelType: 24, want: "google-vertex"},
		{channelType: 33, want: "amazon-bedrock"},
	}
	for _, tt := range tests {
		got, err := tokenSupplyProviderSlug(tt.channelType)
		if err != nil {
			t.Fatalf("tokenSupplyProviderSlug(%d) error = %v", tt.channelType, err)
		}
		if got != tt.want {
			t.Fatalf("tokenSupplyProviderSlug(%d) = %q, want %q", tt.channelType, got, tt.want)
		}
	}
	if _, err := tokenSupplyProviderSlug(999); err == nil {
		t.Fatal("tokenSupplyProviderSlug(999) error = nil, want error")
	}
}
