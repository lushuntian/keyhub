package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"keyhub/internal/database"
)

type modelGapRequest struct {
	ProviderType string `json:"providerType"`
	ModelName    string `json:"modelName"`
	RPMGap       int64  `json:"rpmGap"`
	TPMGap       int64  `json:"tpmGap"`
}

func (s *Server) handleModelGaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := database.ListModelGaps(r.Context(), s.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAdminModelGaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	input, ok := readModelGapInput(w, r)
	if !ok {
		return
	}
	id, err := database.CreateModelGap(r.Context(), s.db, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "model_gap.create", "model_gaps", &id, map[string]any{
		"providerType": input.ProviderType,
		"modelName":    input.ModelName,
		"rpmGap":       input.RPMGap,
		"tpmGap":       input.TPMGap,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (s *Server) handleAdminModelGapAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, ok := modelGapIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "model gap not found")
		return
	}
	input, ok := readModelGapInput(w, r)
	if !ok {
		return
	}
	if err := database.UpdateModelGap(r.Context(), s.db, id, input); err != nil {
		if errors.Is(err, database.ErrModelGapNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = database.InsertAuditLog(r.Context(), s.db, actorFromRequest(r), "model_gap.update", "model_gaps", &id, map[string]any{
		"providerType": input.ProviderType,
		"modelName":    input.ModelName,
		"rpmGap":       input.RPMGap,
		"tpmGap":       input.TPMGap,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func readModelGapInput(w http.ResponseWriter, r *http.Request) (database.ModelGapInput, bool) {
	var request modelGapRequest
	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return database.ModelGapInput{}, false
	}
	request.ProviderType = strings.TrimSpace(request.ProviderType)
	request.ModelName = strings.TrimSpace(request.ModelName)
	switch {
	case request.ProviderType == "":
		writeError(w, http.StatusBadRequest, "providerType is required")
	case len([]rune(request.ProviderType)) > 64:
		writeError(w, http.StatusBadRequest, "providerType must be at most 64 characters")
	case request.ModelName == "":
		writeError(w, http.StatusBadRequest, "modelName is required")
	case len([]rune(request.ModelName)) > 128:
		writeError(w, http.StatusBadRequest, "modelName must be at most 128 characters")
	case request.RPMGap < 0:
		writeError(w, http.StatusBadRequest, "rpmGap must not be negative")
	case request.TPMGap < 0:
		writeError(w, http.StatusBadRequest, "tpmGap must not be negative")
	default:
		return database.ModelGapInput{
			ProviderType: request.ProviderType,
			ModelName:    request.ModelName,
			RPMGap:       request.RPMGap,
			TPMGap:       request.TPMGap,
		}, true
	}
	return database.ModelGapInput{}, false
}

func modelGapIDFromPath(path string) (int64, bool) {
	const prefix = "/api/admin/model-gaps/"
	rawID := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rawID == "" || strings.Contains(rawID, "/") || !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	return id, err == nil && id > 0
}
