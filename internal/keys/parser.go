package keys

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Category struct {
	Code          string   `json:"code"`
	Label         string   `json:"label"`
	NewAPIType    int      `json:"newApiType"`
	DefaultModels []string `json:"defaultModels"`
	KeyFormat     string   `json:"keyFormat"`
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

// ParseLines treats every non-empty line as one opaque credential. Channel
// semantics belong to the receiver platform, so no provider-specific format
// validation or field splitting is performed here.
func ParseLines(rawText string) []ParsedKey {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(rawText, "\r\n", "\n"), "\r", "\n"), "\n")
	items := make([]ParsedKey, 0, len(lines))
	seen := make(map[string]bool)

	for lineIndex, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fingerprint := Fingerprint("", line)
		item := ParsedKey{
			LineNumber:  lineIndex + 1,
			KeyHint:     Mask(line),
			Models:      []string{},
			Fingerprint: fingerprint,
			Normalized:  line,
		}
		if seen[fingerprint] {
			item.Duplicate = true
			item.Error = "本次输入中重复"
		}
		seen[fingerprint] = true
		items = append(items, item)
	}

	return items
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
			parts[i] = maskOne(part)
		}
		return strings.Join(parts, "|")
	}
	return maskOne(value)
}

func KeyFormat(categoryCode string) string {
	return "每行一个密钥"
}

func maskOne(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 8 {
		return fmt.Sprintf("%s****", string(runes[:min(len(runes), 2)]))
	}
	return string(runes[:4]) + "..." + string(runes[len(runes)-4:])
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
