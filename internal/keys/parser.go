package keys

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

type Category struct {
	Code          string   `json:"code"`
	Label         string   `json:"label"`
	NewAPIType    int      `json:"newApiType"`
	DefaultModels []string `json:"defaultModels"`
	KeyFormat     string   `json:"keyFormat"`
}

type ParseOptions struct {
	Category Category
	Models   []string
}

type ParsedKey struct {
	LineNumber   int      `json:"lineNumber"`
	CategoryCode string   `json:"categoryCode"`
	KeyHint      string   `json:"keyHint"`
	Region       string   `json:"region"`
	BaseURL      string   `json:"baseUrl"`
	Models       []string `json:"models"`
	Fingerprint  string   `json:"fingerprint,omitempty"`
	Normalized   string   `json:"-"`
	Error        string   `json:"error,omitempty"`
	Duplicate    bool     `json:"duplicate"`
}

func ParseLines(rawText string, options ParseOptions) []ParsedKey {
	lines := strings.Split(strings.ReplaceAll(rawText, "\r\n", "\n"), "\n")
	items := make([]ParsedKey, 0, len(lines))
	seen := make(map[string]bool)
	models := options.Models
	if len(models) == 0 {
		models = options.Category.DefaultModels
	}

	for lineIndex, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		item := parseLine(line, lineIndex+1, options.Category, models)
		if item.Error == "" {
			if seen[item.Fingerprint] {
				item.Duplicate = true
				item.Error = "本次输入中重复"
			}
			seen[item.Fingerprint] = true
		}
		items = append(items, item)
	}

	return items
}

func parseLine(line string, lineNumber int, category Category, models []string) ParsedKey {
	item := ParsedKey{
		LineNumber:   lineNumber,
		CategoryCode: category.Code,
		Models:       models,
	}

	var parts []string
	if strings.Contains(line, "|") {
		rawParts := strings.Split(line, "|")
		parts = make([]string, 0, len(rawParts))
		for _, part := range rawParts {
			parts = append(parts, strings.TrimSpace(part))
		}
	}

	switch category.Code {
	case "aws_bedrock", "claude_on_aws":
		if len(parts) != 2 && len(parts) != 3 {
			item.Error = "AWS Key 格式应为 AccessKey|SecretKey|Region 或 ApiKey|Region"
			return item
		}
		if hasEmpty(parts) {
			item.Error = "AWS Key 包含空字段"
			return item
		}
		item.Region = parts[len(parts)-1]
		item.Normalized = strings.Join(parts, "|")
	case "azure_openai":
		if len(parts) != 2 && len(parts) != 3 {
			item.Error = "Azure Key 格式应为 Endpoint|ApiKey 或 Endpoint|ApiKey|ApiVersion"
			return item
		}
		if hasEmpty(parts) {
			item.Error = "Azure Key 包含空字段"
			return item
		}
		if _, err := url.ParseRequestURI(parts[0]); err != nil {
			item.Error = "Azure Endpoint 不是有效 URL"
			return item
		}
		item.BaseURL = strings.TrimRight(parts[0], "/")
		item.Normalized = strings.Join(parts, "|")
	default:
		if strings.Contains(line, "|") {
			item.Error = "该分类每行只需要一个 Key"
			return item
		}
		item.Normalized = line
	}

	if len(models) == 0 {
		item.Error = "至少需要选择一个模型"
		return item
	}
	item.Fingerprint = Fingerprint(category.Code, item.Normalized)
	item.KeyHint = Mask(item.Normalized)
	return item
}

func Fingerprint(categoryCode string, normalizedKey string) string {
	sum := sha256.Sum256([]byte(categoryCode + "\x00" + normalizedKey))
	return hex.EncodeToString(sum[:])
}

func Mask(value string) string {
	if value == "" {
		return ""
	}
	if strings.Contains(value, "|") {
		parts := strings.Split(value, "|")
		for i, part := range parts {
			if i == len(parts)-1 && looksLikeRegion(part) {
				continue
			}
			if i == 0 && strings.HasPrefix(strings.ToLower(part), "http") {
				continue
			}
			parts[i] = maskOne(part)
		}
		return strings.Join(parts, "|")
	}
	return maskOne(value)
}

func KeyFormat(categoryCode string) string {
	switch categoryCode {
	case "aws_bedrock", "claude_on_aws":
		return "AccessKey|SecretKey|Region"
	case "azure_openai":
		return "Endpoint|ApiKey|ApiVersion"
	case "anthropic":
		return "sk-ant-..."
	case "openai":
		return "sk-..."
	case "google_ai_studio":
		return "AIza..."
	default:
		return "每行一个 Key"
	}
}

func hasEmpty(parts []string) bool {
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return true
		}
	}
	return false
}

func maskOne(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 8 {
		return fmt.Sprintf("%s****", string(runes[:min(len(runes), 2)]))
	}
	return string(runes[:4]) + "..." + string(runes[len(runes)-4:])
}

func looksLikeRegion(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Count(value, "-") >= 2 && len(value) <= 32
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
