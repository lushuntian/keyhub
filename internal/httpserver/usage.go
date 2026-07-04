package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/newapi"
)

type usageSyncMissingItem struct {
	APIKeyID        int64  `json:"apiKeyId"`
	TargetCode      string `json:"targetCode"`
	CategoryCode    string `json:"categoryCode"`
	KeyHint         string `json:"keyHint"`
	NewAPIChannelID int64  `json:"newApiChannelId"`
}

type usageSyncResult struct {
	Synced          int                    `json:"synced"`
	Baseline        int                    `json:"baseline"`
	Missing         int                    `json:"missing"`
	TotalDeltaQuota int64                  `json:"totalDeltaQuota"`
	TotalDeltaUSD   float64                `json:"totalDeltaUsd"`
	Items           []database.UsageDelta  `json:"items"`
	MissingItems    []usageSyncMissingItem `json:"missingItems"`
}

func (s *Server) handleUsageSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := s.runUsageSync(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "usage.sync", "usage_daily_snapshots", nil, result)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleUsageSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	summary, err := database.LoadUsageSummary(r.Context(), s.db, days, s.keyOwnerFilter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleWorkerRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := database.ListWorkerRuns(r.Context(), s.db, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": runs})
}

func (s *Server) runUsageSync(ctx context.Context) (usageSyncResult, error) {
	targets, err := database.ListUsageSyncTargets(ctx, s.db)
	if err != nil {
		return usageSyncResult{}, err
	}
	aggregationTargets, err := s.loadEnabledAggregationTargets(ctx)
	if err != nil {
		return usageSyncResult{}, err
	}

	usageByTarget, err := s.loadAggregationTargetUsage(ctx, aggregationTargets)
	if err != nil {
		return usageSyncResult{}, err
	}

	readings := make([]database.UsageReading, 0, len(targets))
	result := usageSyncResult{MissingItems: []usageSyncMissingItem{}}
	for _, target := range targets {
		channelByID, ok := usageByTarget[target.TargetCode]
		if !ok {
			result.Missing++
			result.MissingItems = append(result.MissingItems, usageSyncMissingItem{
				APIKeyID:        target.APIKeyID,
				TargetCode:      target.TargetCode,
				CategoryCode:    target.CategoryCode,
				KeyHint:         target.KeyHint,
				NewAPIChannelID: target.NewAPIChannelID,
			})
			continue
		}
		usedQuota, ok := channelByID[target.NewAPIChannelID]
		if !ok {
			result.Missing++
			result.MissingItems = append(result.MissingItems, usageSyncMissingItem{
				APIKeyID:        target.APIKeyID,
				TargetCode:      target.TargetCode,
				CategoryCode:    target.CategoryCode,
				KeyHint:         target.KeyHint,
				NewAPIChannelID: target.NewAPIChannelID,
			})
			continue
		}
		readings = append(readings, database.UsageReading{
			APIKeyID:        target.APIKeyID,
			TargetCode:      target.TargetCode,
			CategoryCode:    target.CategoryCode,
			KeyHint:         target.KeyHint,
			NewAPIChannelID: target.NewAPIChannelID,
			CurrentQuota:    usedQuota,
		})
	}

	deltas, err := database.RecordUsageReadings(ctx, s.db, time.Now(), s.config.NewAPIQuotaPerUSD, readings)
	if err != nil {
		return usageSyncResult{}, err
	}
	result.Items = deltas
	for _, delta := range deltas {
		if delta.Baseline {
			result.Baseline++
			continue
		}
		result.Synced++
		result.TotalDeltaQuota += delta.DeltaQuota
		result.TotalDeltaUSD += delta.DeltaUSD
	}
	return result, nil
}

func (s *Server) loadAggregationTargetUsage(ctx context.Context, targets []config.AggregationTarget) (map[string]map[int64]int64, error) {
	usageByTarget := make(map[string]map[int64]int64, len(targets))
	for _, target := range targets {
		items, err := s.loadSingleAggregationTargetUsage(ctx, target)
		if err != nil {
			return nil, err
		}
		channelByID := make(map[int64]int64, len(items))
		for _, item := range items {
			channelByID[item.ChannelID] = item.UsedQuota
		}
		usageByTarget[target.Code] = channelByID
	}
	return usageByTarget, nil
}

func (s *Server) loadSingleAggregationTargetUsage(ctx context.Context, target config.AggregationTarget) ([]newapi.ChannelUsage, error) {
	switch normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) {
	case aggregationTargetConnectionModeNewAPIReverse:
		client, err := newapi.NewReverseClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
		if err != nil {
			return nil, fmt.Errorf("接收平台 %s new-api 逆向配置无效: %w", target.Code, err)
		}
		items, err := client.ListChannelUsage(ctx)
		if err != nil {
			return nil, fmt.Errorf("接收平台 %s new-api 逆向用量同步失败: %w", target.Code, err)
		}
		return items, nil
	default:
		client, err := newapi.NewPushClient(target.BaseURL, target.Token)
		if err != nil {
			return nil, fmt.Errorf("接收平台 %s 配置无效: %w", target.Code, err)
		}
		items, err := client.ListChannelUsage(ctx)
		if err != nil {
			return nil, fmt.Errorf("接收平台 %s 必须实现 GET /api/keyhub/channels/usage: %w", target.Code, err)
		}
		return items, nil
	}
}
