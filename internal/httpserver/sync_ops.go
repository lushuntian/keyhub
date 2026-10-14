package httpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/newapi"
	"keyhub/internal/security"
	"keyhub/internal/tokensupply"
)

const keyHubPushSource = "keyhub"

func (s *Server) activateKey(ctx context.Context, keyID int64, targetCode string) (int64, string, error) {
	return s.activateKeyWithName(ctx, keyID, targetCode, "")
}

func (s *Server) activateKeyWithName(ctx context.Context, keyID int64, targetCode string, channelName string) (int64, string, error) {
	release, locked, err := database.AcquireAPILock(ctx, s.db, fmt.Sprintf("key-operation:%d", keyID), 5)
	if err != nil {
		return 0, targetCode, err
	}
	if !locked {
		return 0, targetCode, fmt.Errorf("key operation in progress")
	}
	defer release()
	keyRecord, err := database.LoadAPIKeyForSync(ctx, s.db, keyID)
	if err != nil {
		return 0, "", err
	}
	if channelName != "" && keyRecord.Status != "inventory" && keyRecord.Status != "active" {
		return 0, targetCode, fmt.Errorf("key no longer allows API publishing")
	}
	target, err := s.aggregationTarget(ctx, targetCode)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, "", err
	}
	if binding, exists, err := database.GetActiveAPIKeyPushBinding(ctx, s.db, keyID, target.Code); err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, target.Code, err
	} else if exists {
		if channelName != "" {
			if err := database.MarkKeySyncSuccess(ctx, s.db, keyID, binding.RemoteChannelID, "active"); err != nil {
				return 0, target.Code, err
			}
		}
		return binding.RemoteChannelID, target.Code, nil
	}

	encryptor, err := security.NewEncryptor(s.config.EncryptionKey)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, target.Code, err
	}
	plaintextKey, err := encryptor.Decrypt(keyRecord.KeyCiphertext)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, target.Code, err
	}

	payload, err := s.buildTargetChannelPayload(ctx, keyRecord, plaintextKey, target)
	if err != nil {
		_ = database.MarkKeySyncFailure(ctx, s.db, keyID, err.Error())
		return 0, target.Code, err
	}
	payload = applyAggregationTargetChannelSettings(target, payload)
	if channelName != "" {
		payload.Name = channelName
		payload.Remark = nil
	}
	eventPayload := redactedChannelPayload(keyRecord, payload)
	eventPayload["targetCode"] = target.Code
	eventPayload["targetBaseUrl"] = target.BaseURL
	eventPayload["connectionMode"] = normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode)
	eventPayload["channelType"] = normalizeAggregationTargetChannelTypeOrDefault(target.ChannelType)
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
		payload = applyAggregationTargetChannelSettings(target, payload)
		client, err := s.reverseClientForTarget(ctx, target)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		response, err := client.CreateChannel(ctx, payload)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		response.ExternalID = externalID
		return response, nil
	case aggregationTargetConnectionModeTokenSupply:
		client, err := tokensupply.NewClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		providerSlug, err := tokenSupplyProviderSlug(payload.Type)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		description := strings.TrimSpace(payload.Name)
		if payload.Remark != nil && strings.TrimSpace(*payload.Remark) != "" {
			description = strings.TrimSpace(*payload.Remark)
		}
		record, err := client.AddAPIKey(ctx, tokensupply.AddAPIKeyRequest{
			APIKey:        payload.Key,
			ProviderSlug:  providerSlug,
			HasQuotaLimit: 0,
			Description:   description,
			BizDate:       time.Now().Format("20060102"),
		})
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		return newapi.PushChannelResponse{
			ChannelID:  tokensupply.RemoteIDNumber(record.ID),
			Action:     "created",
			ExternalID: externalID,
		}, nil
	default:
		if err := s.validateAggregationTargetCapabilities(ctx, target.BaseURL, target.Token); err != nil {
			return newapi.PushChannelResponse{}, err
		}
		client, err := newapi.NewPushClient(target.BaseURL, target.Token)
		if err != nil {
			return newapi.PushChannelResponse{}, err
		}
		return client.PushChannel(ctx, keyHubPushSource, externalID, payload)
	}
}

func applyAggregationTargetChannelSettings(target config.AggregationTarget, payload newapi.ChannelPayload) newapi.ChannelPayload {
	if normalizeAggregationTargetConnectionModeOrAPI(target.ConnectionMode) == aggregationTargetConnectionModeNewAPIReverse {
		payload.Group = normalizeAggregationTargetReverseGroupName(target.ReverseGroupName)
	}
	return payload
}

func tokenSupplyProviderSlug(channelType int) (string, error) {
	switch channelType {
	case 1:
		return "openai", nil
	case 3:
		return "azure", nil
	case 14:
		return "anthropic", nil
	case 24:
		return "google-vertex", nil
	case 33:
		return "amazon-bedrock", nil
	default:
		return "", fmt.Errorf("tokensupply provider mapping is not configured for channel type %d", channelType)
	}
}

func (s *Server) disableKey(ctx context.Context, keyID int64) error {
	release, locked, lockErr := database.AcquireAPILock(ctx, s.db, fmt.Sprintf("key-operation:%d", keyID), 5)
	if lockErr != nil {
		return lockErr
	}
	if !locked {
		return fmt.Errorf("key operation in progress")
	}
	defer release()
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
		client, err := s.reverseClientForTarget(ctx, target)
		if err != nil {
			return err
		}
		return client.DisableChannel(ctx, remoteChannelID)
	case aggregationTargetConnectionModeTokenSupply:
		client, err := tokensupply.NewClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
		if err != nil {
			return err
		}
		return client.DeleteAPIKeyByRemoteID(ctx, remoteChannelID)
	default:
		if strings.TrimSpace(plaintextKey) == "" {
			return fmt.Errorf("api key plaintext is required for API 接收平台下线")
		}
		payload, err := s.buildTargetChannelPayload(ctx, keyRecord, plaintextKey, target)
		if err != nil {
			return err
		}
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
