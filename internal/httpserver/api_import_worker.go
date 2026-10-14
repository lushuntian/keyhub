package httpserver

import (
	"context"
	"fmt"
	"log"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/tokensupply"
)

func (s *Server) runAPIImportWorker(ctx context.Context) {
	for ctx.Err() == nil {
		worked, err := s.runNextAPIImportTask(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("api import worker: %v", err)
		}
		if !worked || err != nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (s *Server) runNextAPIImportTask(ctx context.Context) (bool, error) {
	release, locked, err := database.AcquireAPILock(ctx, s.db, "api-import-worker", 0)
	if err != nil || !locked {
		return false, err
	}
	defer release()
	task, err := database.ClaimAPIImportTask(ctx, s.db)
	if err != nil || task == nil {
		return false, err
	}
	started := time.Now()
	taskCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	status, code := s.executeAPIImportTask(taskCtx, *task)
	cancel()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cleanupCancel()
	if err := database.FinishAPIImportTask(cleanupCtx, s.db, task.ID, status, code); err != nil {
		return true, err
	}
	detail := map[string]any{"jobId": task.JobID, "taskId": task.ID, "status": status, "errorCode": code}
	_ = database.InsertWorkerRun(cleanupCtx, s.db, "api_publish", status, started, time.Now(), detail, code)
	_ = database.InsertAuditLog(cleanupCtx, s.db, "worker", "api.keys.publish", "api_import_tasks", &task.ID, detail)
	return true, nil
}

func apiImportChannelName(task database.APIImportTask) string {
	return fmt.Sprintf("keyhub_api_%s_%d", task.JobID, task.ID)
}

func (s *Server) executeAPIImportTask(ctx context.Context, task database.APIImportTask) (string, string) {
	failureStatus := "failed"
	if task.ReconcileOnly {
		failureStatus = "needs_review"
	}
	username, err := database.APIImportOwner(ctx, s.db, task.UserID)
	if err != nil {
		return failureStatus, "OWNER_UNAVAILABLE"
	}
	if task.KeyID == 0 {
		return failureStatus, "KEY_UNAVAILABLE"
	}
	allowed, err := database.UserCanUseAggregationTarget(ctx, s.db, username, task.TargetCode)
	if err != nil || !allowed {
		return failureStatus, "CHANNEL_NOT_ALLOWED"
	}
	key, err := database.LoadAPIKeyForSync(ctx, s.db, task.KeyID)
	if err != nil || key.CreatedBy != username || (key.Status != "active" && key.Status != "inventory") {
		return failureStatus, "KEY_UNAVAILABLE"
	}
	target, err := s.aggregationTarget(ctx, task.TargetCode)
	if err != nil {
		return failureStatus, "TARGET_UNAVAILABLE"
	}
	if apiTargetSignature(target) != task.TargetSignature {
		return failureStatus, "TARGET_CHANGED"
	}
	if task.ReconcileOnly {
		if s.reconcileAPIImportTask(ctx, task, target) {
			return "succeeded", ""
		}
		return "needs_review", "REMOTE_OUTCOME_UNKNOWN"
	}
	// Validate every local requirement before beginning the non-transactional call.
	plaintext, err := s.decryptKeyPlaintext(key)
	if err != nil {
		return "failed", "KEY_UNAVAILABLE"
	}
	if _, err := s.buildTargetChannelPayload(ctx, key, plaintext, target); err != nil {
		return "failed", "TARGET_CONFIGURATION_INVALID"
	}
	if _, _, err := s.activateKeyWithName(ctx, task.KeyID, task.TargetCode, apiImportChannelName(task)); err == nil {
		return "succeeded", ""
	}
	// Use a fresh bounded context even if the outbound request timed out.
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if s.reconcileAPIImportTask(reconcileCtx, task, target) {
		return "succeeded", ""
	}
	return "needs_review", "REMOTE_OUTCOME_UNKNOWN"
}

func (s *Server) reconcileAPIImportTask(ctx context.Context, task database.APIImportTask, target config.AggregationTarget) bool {
	release, locked, err := database.AcquireAPILock(ctx, s.db, fmt.Sprintf("key-operation:%d", task.KeyID), 5)
	if err != nil || !locked {
		return false
	}
	defer release()
	key, err := database.LoadAPIKeyForSync(ctx, s.db, task.KeyID)
	if err != nil || (key.Status != "inventory" && key.Status != "active") {
		return false
	}
	if binding, exists, err := database.GetActiveAPIKeyPushBinding(ctx, s.db, task.KeyID, task.TargetCode); err != nil {
		return false
	} else if exists {
		return database.MarkKeySyncSuccess(ctx, s.db, task.KeyID, binding.RemoteChannelID, "active") == nil
	}
	var channelID int64
	switch target.ConnectionMode {
	case aggregationTargetConnectionModeNewAPIReverse:
		client, err := s.reverseClientForTarget(ctx, target)
		if err != nil {
			return false
		}
		channel, err := client.FindUniqueChannelByName(ctx, apiImportChannelName(task))
		if err != nil || channel == nil {
			return false
		}
		channelID = channel.ID
	case aggregationTargetConnectionModeTokenSupply:
		client, err := tokensupply.NewClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
		if err != nil {
			return false
		}
		records, err := client.ListAPIKeys(ctx)
		if err != nil {
			return false
		}
		for _, record := range records {
			if record.Description != apiImportChannelName(task) {
				continue
			}
			if channelID != 0 {
				return false
			}
			channelID = tokensupply.RemoteIDNumber(record.ID)
		}
	default:
		// The generic contract has no external-id lookup. Never guess or re-create.
		return false
	}
	if channelID <= 0 {
		return false
	}
	if err := database.UpsertAPIKeyPushBinding(ctx, s.db, task.KeyID, task.TargetCode, channelID, "active"); err != nil {
		return false
	}
	if err := database.MarkKeySyncSuccess(ctx, s.db, task.KeyID, channelID, "active"); err != nil {
		return false
	}
	_ = database.InsertSyncEvent(ctx, s.db, &task.KeyID, "activate", "success", map[string]any{"source": "api_reconciliation", "targetCode": task.TargetCode}, map[string]any{"newApiChannelId": channelID}, "")
	return true
}
