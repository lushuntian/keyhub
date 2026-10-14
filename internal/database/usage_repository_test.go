package database

import (
	"database/sql"
	"math"
	"testing"
)

func TestUsageUSD(t *testing.T) {
	tests := []struct {
		name        string
		quota       int64
		quotaPerUSD float64
		want        float64
	}{
		{name: "standard units", quota: 1_000_000, quotaPerUSD: 500_000, want: 2},
		{name: "custom units", quota: 1_000_000, quotaPerUSD: 250_000, want: 4},
		{name: "zero usage", quota: 0, quotaPerUSD: 500_000, want: 0},
		{name: "default units", quota: 1_000_000, quotaPerUSD: 0, want: 2},
		{name: "invalid units", quota: 1_000_000, quotaPerUSD: -1, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := usageUSD(tt.quota, tt.quotaPerUSD)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("usageUSD() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncrementalUsageQuota(t *testing.T) {
	tests := []struct {
		name     string
		current  int64
		previous sql.NullInt64
		want     int64
	}{
		{name: "baseline is not new usage", current: 500, previous: sql.NullInt64{}, want: 0},
		{name: "positive increase", current: 650, previous: sql.NullInt64{Int64: 500, Valid: true}, want: 150},
		{name: "unchanged", current: 500, previous: sql.NullInt64{Int64: 500, Valid: true}, want: 0},
		{name: "reset is not new usage", current: 100, previous: sql.NullInt64{Int64: 500, Valid: true}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := incrementalUsageQuota(tt.current, tt.previous); got != tt.want {
				t.Fatalf("incrementalUsageQuota(%d, %+v) = %d, want %d", tt.current, tt.previous, got, tt.want)
			}
		})
	}
}
