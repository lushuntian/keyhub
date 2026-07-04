package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/database"
	"keyhub/internal/httpserver"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if cfg.AutoMigrate {
		if err := database.ApplyMigrations(ctx, db, cfg.MigrationsDir); err != nil {
			log.Fatalf("migrations: %v", err)
		}
	}
	if cfg.AuthEnabled {
		adminCount, err := database.CountAdminUsers(ctx, db)
		if err != nil {
			log.Fatalf("count admin users: %v", err)
		}
		if adminCount == 0 && strings.TrimSpace(cfg.BootstrapPassword) == "" {
			if !cfg.RegistrationEnabled {
				log.Fatal("no admin users exist; set KEYHUB_BOOTSTRAP_ADMIN_PASSWORD or enable KEYHUB_REGISTRATION_ENABLED")
			}
			log.Print("no admin users exist; registration is enabled for the first account")
		} else {
			created, err := database.EnsureBootstrapAdmin(ctx, db, cfg.BootstrapAdmin, cfg.BootstrapPassword)
			if err != nil {
				log.Fatalf("bootstrap admin: %v", err)
			}
			if created {
				log.Printf("bootstrap admin %q created", cfg.BootstrapAdmin)
			}
		}
	}

	app := httpserver.New(cfg, db)
	app.StartWorkers(ctx)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("keyhub listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
