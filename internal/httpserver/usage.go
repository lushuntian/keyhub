package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/newapi"
	"keyhub/internal/tokensupply"
)

type usageSyncMissingItem struct {
	APIKeyID        int64  `json:"apiKeyId"`
	TargetCode      string `json:"targetCode"`
	CategoryCode    string `json:"categoryCode"`
	KeyHint         string `json:"keyHint"`
	NewAPIChannelID int64  `json:"newApiChannelId"`
}

type usageSyncTargetFailure struct {
	ConnectionMode string `json:"connectionMode"`
	Error          string `json:"error"`
}

type usageSyncResult struct {
	Synced              int                        `json:"synced"`
	Baseline            int                        `json:"baseline"`
	Missing             int                        `json:"missing"`
	RemoteStatusChecked int                        `json:"remoteStatusChecked"`
	RemoteStatusMissing int                        `json:"remoteStatusMissing"`
	FailedTargets       int                        `json:"failedTargets"`
	TotalSyncedQuota    int64                      `json:"totalSyncedQuota"`
	TotalSyncedUSD      float64                    `json:"totalSyncedUsd"`
	TotalDeltaQuota     int64                      `json:"totalDeltaQuota"`
	TotalDeltaUSD       float64                    `json:"totalDeltaUsd"`
	Items               []database.UsageSyncRecord `json:"items"`
	MissingItems        []usageSyncMissingItem     `json:"missingItems"`
	TargetFailures      []usageSyncTargetFailure   `json:"targetFailures"`
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
	if s.isRegularUserRequest(r) {
		summary.TotalQuota = 0
		for index := range summary.ByDay {
			summary.ByDay[index].Quota = 0
		}
		for index := range summary.Categories {
			summary.Categories[index].Quota = 0
		}
		for index := range summary.Channels {
			summary.Channels[index].Quota = 0
		}
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

	usageByTarget, failedTargetCodes, targetFailures, err := s.loadAggregationTargetUsage(ctx, aggregationTargets)
	if err != nil {
		return usageSyncResult{}, err
	}

	reverseTargetCodes := make(map[string]struct{})
	for _, target := range aggregationTargets {
		if normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) == aggregationTargetConnectionModeNewAPIReverse {
			reverseTargetCodes[target.Code] = struct{}{}
		}
	}

	readings := make([]database.UsageReading, 0, len(targets))
	statusReadings := make([]database.RemoteChannelStatusReading, 0, len(targets))
	result := usageSyncResult{
		FailedTargets:  len(targetFailures),
		MissingItems:   []usageSyncMissingItem{},
		TargetFailures: targetFailures,
	}
	for _, target := range targets {
		if _, failed := failedTargetCodes[target.TargetCode]; failed {
			continue
		}
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
		channel, channelExists := channelByID[target.NewAPIChannelID]
		if _, isReverseTarget := reverseTargetCodes[target.TargetCode]; isReverseTarget {
			var remoteStatus *int
			if channelExists {
				value := channel.Status
				remoteStatus = &value
			} else {
				result.RemoteStatusMissing++
			}
			statusReadings = append(statusReadings, database.RemoteChannelStatusReading{
				APIKeyID:        target.APIKeyID,
				TargetCode:      target.TargetCode,
				NewAPIChannelID: target.NewAPIChannelID,
				RemoteStatus:    remoteStatus,
			})
		}
		if !channelExists {
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
			CurrentQuota:    channel.UsedQuota,
		})
	}

	syncedAt := time.Now()
	result.RemoteStatusChecked, err = database.RecordRemoteChannelStatuses(ctx, s.db, syncedAt, statusReadings)
	if err != nil {
		return usageSyncResult{}, err
	}
	records, err := database.RecordUsageReadings(ctx, s.db, syncedAt, s.config.NewAPIQuotaPerUSD, readings)
	if err != nil {
		return usageSyncResult{}, err
	}
	result.Items = records
	for _, record := range records {
		result.Synced++
		if record.Baseline {
			result.Baseline++
		}
		result.TotalSyncedQuota += record.SyncedQuota
		result.TotalSyncedUSD += record.SyncedUSD
		result.TotalDeltaQuota += record.DeltaQuota
		result.TotalDeltaUSD += record.DeltaUSD
	}
	return result, nil
}

func (s *Server) loadAggregationTargetUsage(ctx context.Context, targets []config.AggregationTarget) (map[string]map[int64]newapi.ChannelUsage, map[string]struct{}, []usageSyncTargetFailure, error) {
	usageByTarget := make(map[string]map[int64]newapi.ChannelUsage, len(targets))
	failedTargetCodes := make(map[string]struct{})
	targetFailures := make([]usageSyncTargetFailure, 0)
	for _, target := range targets {
		items, err := s.loadSingleAggregationTargetUsage(ctx, target)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, nil, ctx.Err()
			}
			failedTargetCodes[target.Code] = struct{}{}
			targetFailures = append(targetFailures, usageSyncTargetFailure{
				ConnectionMode: normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode),
				Error:          err.Error(),
			})
			continue
		}
		channelByID := make(map[int64]newapi.ChannelUsage, len(items))
		for _, item := range items {
			channelByID[item.ChannelID] = item
		}
		usageByTarget[target.Code] = channelByID
	}
	return usageByTarget, failedTargetCodes, targetFailures, nil
}

func (s *Server) loadSingleAggregationTargetUsage(ctx context.Context, target config.AggregationTarget) ([]newapi.ChannelUsage, error) {
	switch normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) {
	case aggregationTargetConnectionModeNewAPIReverse:
		client, err := s.reverseClientForTarget(ctx, target)
		if err != nil {
			return nil, aggregationTargetUsageError("new-api 逆向用量同步配置无效", target, err)
		}
		items, err := client.ListChannelUsage(ctx)
		if err != nil {
			return nil, aggregationTargetUsageError("new-api 逆向用量同步失败", target, err)
		}
		return items, nil
	case aggregationTargetConnectionModeTokenSupply:
		client, err := tokensupply.NewClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
		if err != nil {
			return nil, aggregationTargetUsageError("tokensupply 用量同步配置无效", target, err)
		}
		usages, err := client.ListUsage(ctx, s.config.NewAPIQuotaPerUSD)
		if err != nil {
			return nil, aggregationTargetUsageError("tokensupply 用量同步失败", target, err)
		}
		items := make([]newapi.ChannelUsage, 0, len(usages))
		for _, usage := range usages {
			items = append(items, newapi.ChannelUsage{
				ChannelID: usage.RemoteID,
				UsedQuota: usage.UsedQuota,
			})
		}
		return items, nil
	default:
		client, err := newapi.NewPushClient(target.BaseURL, target.Token)
		if err != nil {
			return nil, aggregationTargetUsageError("用量同步配置无效", target, err)
		}
		items, err := client.ListChannelUsage(ctx)
		if err != nil {
			return nil, aggregationTargetUsageError("用量同步失败，必须实现 GET /api/keyhub/channels/usage", target, err)
		}
		return items, nil
	}
}

func aggregationTargetUsageError(message string, target config.AggregationTarget, err error) error {
	detail := sanitizeAggregationTargetError(err, target)
	if detail == "" {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("%s: %s", message, detail)
}

func sanitizeAggregationTargetError(err error, target config.AggregationTarget) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return ""
	}

	values := []string{
		target.Code,
		target.Name,
		target.BaseURL,
		target.Token,
		target.ReverseUsername,
		target.ReversePassword,
	}
	if parsed, parseErr := url.Parse(target.BaseURL); parseErr == nil {
		values = append(values, parsed.Host, parsed.Hostname())
	}
	sort.SliceStable(values, func(i, j int) bool {
		return len(values[i]) > len(values[j])
	})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		message = strings.ReplaceAll(message, value, "[已隐藏]")
	}
	return message
}
