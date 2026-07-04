package httpserver

import (
	"context"
	"time"

	"keyhub/internal/database"
)

type workerAction func(context.Context) (any, error)

func (s *Server) StartWorkers(ctx context.Context) {
	if !s.config.WorkerEnabled {
		return
	}
	s.startWorker(ctx, "health_check", s.config.HealthCheckEvery, func(ctx context.Context) (any, error) {
		return s.runHealthChecks(ctx, true, 200)
	})
	s.startWorker(ctx, "usage_sync", s.config.UsageSyncEvery, func(ctx context.Context) (any, error) {
		return s.runUsageSync(ctx)
	})
}

func (s *Server) startWorker(ctx context.Context, name string, interval time.Duration, action workerAction) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.executeWorker(ctx, name, interval, action)
			}
		}
	}()
}

func (s *Server) executeWorker(ctx context.Context, name string, interval time.Duration, action workerAction) {
	started := time.Now()
	timeout := interval
	if timeout < time.Minute {
		timeout = time.Minute
	}
	if timeout > 30*time.Minute {
		timeout = 30 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	detail, err := action(runCtx)
	finished := time.Now()
	status := "success"
	errorMessage := ""
	if err != nil {
		status = "failed"
		errorMessage = err.Error()
		detail = map[string]any{"error": errorMessage}
	}
	_ = database.InsertWorkerRun(context.Background(), s.db, name, status, started, finished, detail, errorMessage)
	_ = database.InsertAuditLog(context.Background(), s.db, "worker", "worker.run", name, nil, map[string]any{
		"status":     status,
		"startedAt":  started,
		"finishedAt": finished,
		"error":      errorMessage,
	})
}
