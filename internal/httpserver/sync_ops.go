package httpserver

import (
	"context"
	"fmt"
	"strings"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/newapi"
	"keyhub/internal/security"
)

const keyHubPushSource = "keyhub"

func (s *Server) activateKey(ctx context.Context, keyID int64, targetCode string) (int64, string, error) {
	keyRecord, err := database.LoadAPIKeyForSync(ctx, s.db, keyID)
	if err != nil {
		return 0, "", err
	}
	if keyRecord.Status == "active" && keyRecord.NewAPIChannelID.Valid {
		return keyRecord.NewAPIChannelID.Int64, "", nil
	}

	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, "", err
	}
	plaintextKey, err := encryptor.Decrypt(keyRecord.KeyCiphertext)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, "", err
	}
	target, err := s.aggregationTarget(ctx, targetCode)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, "", err
	}

	payload := buildNewAPIChannelPayload(keyRecord, plaintextKey)
	eventPayload := redactedChannelPayload(keyRecord, payload)
	eventPayload["targetCode"] = target.Code
	eventPayload["targetBaseUrl"] = target.BaseURL
	eventPayload["connectionMode"] = normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode)
	externalID := fmt.Sprintf("api_key:%d", keyID)
	if err := database.InsertSyncEvent(ctx, s.db, &keyID, "activate", "pending", eventPayload, nil, ""); err != nil {
		return 0, target.Code, err
	}
	response, err := s.createTargetChannel(ctx, target, externalID, payload)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		_ = database.InsertSyncEvent(ctx, s.db, &keyID, "activate", "failed", eventPayload, nil, err.Error())
		return 0, target.Code, err
	}
	if response.ChannelID <= 0 {
		err := fmt.Errorf("new-api keyhub push returned invalid channel id")
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		_ = database.InsertSyncEvent(ctx, s.db, &keyID, "activate", "failed", eventPayload, nil, err.Error())
		return 0, target.Code, err
	}
	if err := database.UpsertAPIKeyPushBinding(ctx, s.db, keyID, target.Code, response.ChannelID, "active"); err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		_ = database.InsertSyncEvent(ctx, s.db, &keyID, "activate", "failed", eventPayload, nil, err.Error())
		return 0, target.Code, err
	}
	if err := database.MarkKeySyncSuccess(ctx, s.db, keyID, response.ChannelID, "active"); err != nil {
		return 0, target.Code, err
	}
	_ = database.InsertSyncEvent(ctx, s.db, &keyID, "activate", "success", eventPayload, map[string]any{
		"newApiChannelId": response.ChannelID,
		"targetCode":      target.Code,
		"action":          response.Action,
	}, "")
	return response.ChannelID, target.Code, nil
}

func (s *Server) createTargetChannel(ctx context.Context, target config.AggregationTarget, externalID string, payload newapi.ChannelPayload) (newapi.PushChannelResponse, error) {
	switch normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) {
	case aggregationTargetConnectionModeNewAPIReverse:
		client, err := newapi.NewReverseClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		response, err := client.CreateChannel(ctx, payload)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		response.ExternalID = externalID
		return response, nil
	default:
		if err := s.validateAggregationTargetCapabilities(ctx, target.Code, target.BaseURL, target.Token); err != nil {
			return newapi.PushChannelResponse{}, err
		}
		client, err := newapi.NewPushClient(target.BaseURL, target.Token)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		return client.PushChannel(ctx, keyHubPushSource, externalID, payload)
	}
}

func (s *Server) disableKey(ctx context.Context, keyID int64) error {
	keyRecord, err := database.LoadAPIKeyForSync(ctx, s.db, keyID)
	if err != nil {
		return err
	}
	eventPayload := map[string]any{
		"apiKeyId":     keyID,
		"action":       "disable",
		"categoryCode": keyRecord.CategoryCode,
		"keyHint":      keyRecord.KeyHint,
	}
	bindings, err := database.ListAPIKeyPushBindings(ctx, s.db, keyID)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return err
	}
	if len(bindings) > 0 {
		eventPayload["bindings"] = bindings
		if err := database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "pending", eventPayload, nil, ""); err != nil {
			return err
		}
		disabledBindings := make([]map[string]any, 0, len(bindings))
		var plaintextKey string
		var hasPlaintextKey bool
		for _, binding := range bindings {
			target, err := s.aggregationTarget(ctx, binding.TargetCode)
			if err != nil {
				_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
				_ = database.MarkAPIKeyPushBindingFailure(ctx, s.db, keyID, binding.TargetCode, err.Error())
				_ = database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "failed", eventPayload, nil, err.Error())
				return err
			}
			if normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) == aggregationTargetConnectionModeAPI && !hasPlaintextKey {
				plaintextKey, err = s.decryptKeyPlaintext(keyRecord)
				if err != nil {
					_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
					_ = database.MarkAPIKeyPushBindingFailure(ctx, s.db, keyID, binding.TargetCode, err.Error())
					_ = database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "failed", eventPayload, nil, err.Error())
					return err
				}
				hasPlaintextKey = true
			}
			if err := s.disableTargetChannel(ctx, keyRecord, target, binding.RemoteChannelID, plaintextKey); err != nil {
				_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
				_ = database.MarkAPIKeyPushBindingFailure(ctx, s.db, keyID, binding.TargetCode, err.Error())
				_ = database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "failed", eventPayload, nil, err.Error())
				return err
			}
			if err := database.MarkAPIKeyPushBindingDisabled(ctx, s.db, keyID, binding.TargetCode); err != nil {
				_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
				_ = database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "failed", eventPayload, nil, err.Error())
				return err
			}
			disabledBindings = append(disabledBindings, map[string]any{
				"targetCode":      binding.TargetCode,
				"remoteChannelId": binding.RemoteChannelID,
			})
		}
		if err := database.MarkKeyDisabled(ctx, s.db, keyID); err != nil {
			return err
		}
		_ = database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "success", eventPayload, map[string]any{
			"disabled": true,
			"bindings": disabledBindings,
		}, "")
		return nil
	}
	if keyRecord.NewAPIChannelID.Valid {
		eventPayload["newApiChannelId"] = keyRecord.NewAPIChannelID.Int64
		client, err := s.newAPIClient()
		if err != nil {
			_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
			return err
		}
		if err := database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "pending", eventPayload, nil, ""); err != nil {
			return err
		}
		if err := client.DisableChannel(ctx, keyRecord.NewAPIChannelID.Int64); err != nil {
			_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
			_ = database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "failed", eventPayload, nil, err.Error())
			return fmt.Errorf("disable new-api channel %d: %w", keyRecord.NewAPIChannelID.Int64, err)
		}
	}
	if err := database.MarkKeyDisabled(ctx, s.db, keyID); err != nil {
		return err
	}
	_ = database.InsertSyncEvent(ctx, s.db, &keyID, "disable", "success", eventPayload, map[string]any{"disabled": true}, "")
	return nil
}

func (s *Server) decryptKeyPlaintext(keyRecord database.APIKeyForSync) (string, error) {
	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		return "", err
	}
	return encryptor.Decrypt(keyRecord.KeyCiphertext)
}

func (s *Server) disableTargetChannel(ctx context.Context, keyRecord database.APIKeyForSync, target config.AggregationTarget, remoteChannelID int64, plaintextKey string) error {
	switch normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) {
	case aggregationTargetConnectionModeNewAPIReverse:
		client, err := newapi.NewReverseClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
		if err != nil {
			return err
		}
		return client.DisableChannel(ctx, remoteChannelID)
	default:
		if strings.TrimSpace(plaintextKey) == "" {
			return fmt.Errorf("api key plaintext is required for API 接收平台下线")
		}
		payload := buildNewAPIChannelPayload(keyRecord, plaintextKey)
		payload.ID = remoteChannelID
		client, err := newapi.NewPushClient(target.BaseURL, target.Token)
		if err != nil {
			return err
		}
		response, err := client.PushChannel(ctx, keyHubPushSource, fmt.Sprintf("api_key:%d", keyRecord.ID), payload)
		if err != nil {
			return err
		}
		if response.ChannelID <= 0 {
			return fmt.Errorf("new-api keyhub disable returned invalid channel id")
		}
		return nil
	}
}
