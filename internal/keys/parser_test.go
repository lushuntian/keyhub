package keys

import "testing"

func TestParseAWSLines(t *testing.T) {
	category := Category{
		Code:          "aws_bedrock",
		DefaultModels: []string{"claude-opus-4-8"},
	}

	items := ParseLines("AKIA1234|secret-value|us-east-1", ParseOptions{Category: category})
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Error != "" {
		t.Fatalf("unexpected error: %s", items[0].Error)
	}
	if items[0].Region != "us-east-1" {
		t.Fatalf("region = %q, want us-east-1", items[0].Region)
	}
	if items[0].Fingerprint == "" {
		t.Fatal("fingerprint should be set")
	}
}

func TestParseAzureLine(t *testing.T) {
	category := Category{
		Code:          "azure_openai",
		DefaultModels: []string{"gpt-4.1"},
	}

	items := ParseLines("https://example.openai.azure.com|secret|2024-12-01-preview", ParseOptions{Category: category})
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Error != "" {
		t.Fatalf("unexpected error: %s", items[0].Error)
	}
	if items[0].BaseURL != "https://example.openai.azure.com" {
		t.Fatalf("baseURL = %q", items[0].BaseURL)
	}
}

func TestParseDuplicateInput(t *testing.T) {
	category := Category{
		Code:          "openai",
		DefaultModels: []string{"gpt-4.1"},
	}

	items := ParseLines("sk-test\nsk-test", ParseOptions{Category: category})
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if !items[1].Duplicate {
		t.Fatal("second item should be duplicate")
	}
}
