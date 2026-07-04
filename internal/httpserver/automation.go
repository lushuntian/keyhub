package httpserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"keyhub/internal/database"
)

type healthRunRequest struct {
	AutoDisable *bool `json:"autoDisable"`
	Limit       int   `json:"limit"`
}

type healthRunResult struct {
	APIKeyID        int64   `json:"apiKeyId"`
	NewAPIChannelID int64   `json:"newApiChannelId"`
	Success         bool    `json:"success"`
	LatencyMS       int     `json:"latencyMs"`
	Message         string  `json:"message"`
	ErrorCode       string  `json:"errorCode,omitempty"`
	AutoDisabled    bool    `json:"autoDisabled"`
	DisableError    string  `json:"disableError,omitempty"`
	DurationSeconds float64 `json:"durationSeconds"`
}

func (s *Server) handleHealthChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	checks, err := database.ListHealthChecks(r.Context(), s.db, limit, s.keyOwnerFilter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": checks})
}

func (s *Server) handleHealthCheckRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	request := healthRunRequest{Limit: 200}
	if r.Body != nil {
		decodeErr := readJSON(w, r, &request)
		if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
			writeError(w, http.StatusBadRequest, decodeErr.Error())
			return
		}
		if request.Limit == 0 {
			request.Limit = 200
		}
		if err := validateHealthRunRequest(request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	autoDisable := true
	if request.AutoDisable != nil {
		autoDisable = *request.AutoDisable
	}
	results, err := s.runHealthChecks(r.Context(), autoDisable, request.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "health_checks.run", "api_keys", nil, map[string]any{
		"autoDisable": autoDisable,
		"count":       len(results),
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": results})
}

func validateHealthRunRequest(request healthRunRequest) error {
	if request.Limit < 0 {
		return errors.New("limit must be greater than or equal to 0")
	}
	return nil
}

func (s *Server) runHealthChecks(ctx context.Context, autoDisable bool, limit int) ([]healthRunResult, error) {
	client, err := s.newAPIClient()
	if err != nil {
		return nil, err
	}
	targets, err := database.ListHealthCheckTargets(ctx, s.db, limit)
	if err != nil {
		return nil, err
	}

	results := make([]healthRunResult, 0, len(targets))
	for _, target := range targets {
		started := time.Now()
		testResult, err := client.TestChannel(ctx, target.NewAPIChannelID)
		elapsedMS := int(time.Since(started).Milliseconds())
		result := healthRunResult{
			APIKeyID:        target.ID,
			NewAPIChannelID: target.NewAPIChannelID,
			LatencyMS:       elapsedMS,
			DurationSeconds: time.Since(started).Seconds(),
		}
		if testResult.Time > 0 {
			result.LatencyMS = int(testResult.Time * 1000)
		}
		if err != nil {
			result.Success = false
			result.Message = err.Error()
			_ = database.InsertHealthCheck(ctx, s.db, target.ID, "failed", result.LatencyMS, "", result.Message)
			results = append(results, result)
			continue
		}
		result.Success = testResult.Success
		result.Message = testResult.Message
		result.ErrorCode = testResult.ErrorCode
		status := "success"
		if !testResult.Success {
			status = "failed"
			if result.Message == "" {
				result.Message = "new-api channel test failed"
			}
		}
		_ = database.InsertHealthCheck(ctx, s.db, target.ID, status, result.LatencyMS, result.ErrorCode, result.Message)
		if !testResult.Success && autoDisable {
			if err := s.disableKey(ctx, target.ID); err != nil {
				result.DisableError = err.Error()
			} else {
				result.AutoDisabled = true
			}
		}
		results = append(results, result)
	}
	return results, nil
}
