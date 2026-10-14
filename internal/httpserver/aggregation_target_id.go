package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newAggregationTargetCode() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate aggregation target ID: %w", err)
	}
	return "target_" + hex.EncodeToString(id[:]), nil
}
