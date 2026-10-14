package httpserver

import (
	"regexp"
	"testing"
)

func TestNewAggregationTargetCodeCreatesOpaqueValidID(t *testing.T) {
	code, err := newAggregationTargetCode()
	if err != nil {
		t.Fatalf("newAggregationTargetCode returned error: %v", err)
	}
	if !regexp.MustCompile(`^target_[0-9a-f]{32}$`).MatchString(code) {
		t.Fatalf("newAggregationTargetCode = %q, want target_ followed by 32 lowercase hex characters", code)
	}
}
