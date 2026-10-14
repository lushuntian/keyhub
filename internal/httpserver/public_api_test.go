package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"keyhub/internal/config"
)

func TestPublicImportValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  publicKeyImportRequest
		valid    bool
		expanded int
	}{
		{"ordinary", publicKeyImportRequest{Keys: []string{" first ", "second"}, ChannelTypes: []string{"anthropic"}}, true, 2},
		{"requires selection", publicKeyImportRequest{Keys: []string{"first"}}, false, 0},
		{"empty key", publicKeyImportRequest{Keys: []string{" "}, ChannelTypes: []string{"anthropic"}}, false, 0},
		{"multiline", publicKeyImportRequest{Keys: []string{"first\nsecond"}, ChannelTypes: []string{"anthropic"}}, false, 0},
		{"unknown type", publicKeyImportRequest{Keys: []string{"first"}, ChannelTypes: []string{"unknown"}}, false, 0},
		{"aws type required", publicKeyImportRequest{Keys: []string{"ak|sk|us-east-1"}, ChannelTypes: []string{"aws_bedrock"}}, false, 0},
		{"legacy aws", publicKeyImportRequest{Keys: []string{"ak|sk|us-east-1"}, ChannelTypes: []string{"aws_bedrock"}, AWSCredentialType: "ak_sk"}, true, 1},
		{"modern regions", publicKeyImportRequest{Keys: []string{"first", "first"}, ChannelTypes: []string{"aws_bedrock_new"}, AWSBedrockRegions: []string{"https://bedrock-mantle.us-east-1.api.aws", "https://bedrock-mantle.us-west-2.api.aws"}}, true, 4},
		{"modern mixing", publicKeyImportRequest{Keys: []string{"first"}, ChannelTypes: []string{"aws_bedrock_new", "anthropic"}}, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := normalizePublicImport(&tt.request)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v, err=%v", tt.valid, err)
			}
			if tt.valid && len(items) != tt.expanded {
				t.Fatalf("got %d items", len(items))
			}
		})
	}
	request := publicKeyImportRequest{Keys: make([]string, 51), ChannelTypes: []string{"aws_bedrock_new"}, AWSBedrockRegions: []string{"https://bedrock-mantle.a", "https://bedrock-mantle.b"}}
	if _, err := normalizePublicImport(&request); err == nil {
		t.Fatal("must reject excessive expansion")
	}
}

func TestPublicAPIRejectsCookieAndMalformedToken(t *testing.T) {
	s := New(config.Config{AuthEnabled: true}, nil)
	for _, header := range []string{"", "Bearer abc", "Basic user:password", "Bearer kh_invalid"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/key-options", nil)
		request.Header.Set("Authorization", header)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "cookie-is-not-api-auth"})
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != 401 {
			t.Fatalf("status=%d", response.Code)
		}
	}
}

func TestPublicAPIRequiresAuthEvenInDevelopment(t *testing.T) {
	s := New(config.Config{}, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/key-options", nil))
	if response.Code != 503 {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestPublicJSONRejectsExtraValuesAndUnknownFields(t *testing.T) {
	for _, body := range []string{`{"keys":[]} {}`, `{"owner":"another-user"}`, `{"keys":[]} trailing`} {
		var request publicKeyImportRequest
		if err := readAPIJSON(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(body)), &request); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestPublicImportHashIncludesPayloadAndSecret(t *testing.T) {
	request := publicKeyImportRequest{Keys: []string{"one"}, ChannelTypes: []string{"anthropic"}}
	first := publicImportHash(request, "secret")
	if first != publicImportHash(request, "secret") {
		t.Fatal("hash not stable")
	}
	if first == publicImportHash(request, "other") {
		t.Fatal("hash ignores secret")
	}
	request.Keys[0] = "two"
	if first == publicImportHash(request, "secret") {
		t.Fatal("hash ignores payload")
	}
	encoded, _ := json.Marshal(request)
	if strings.Contains(first, string(encoded)) {
		t.Fatal("request leaked")
	}
}

func TestOpenAPIImportExamplesAccepted(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]struct {
			Post struct {
				RequestBody struct {
					Content map[string]struct {
						Examples map[string]struct{ Value publicKeyImportRequest }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	examples := spec.Paths["/key-imports"].Post.RequestBody.Content["application/json"].Examples
	if len(examples) != 6 {
		t.Fatalf("want six channel examples, got %d", len(examples))
	}
	for name, example := range examples {
		if _, err := normalizePublicImport(&example.Value); err != nil {
			t.Errorf("documented example %s rejected: %v", name, err)
		}
	}
}
