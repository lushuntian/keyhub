package httpserver

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"keyhub/internal/config"
	"keyhub/internal/newapi"
)

type reverseClientCache struct {
	mu      sync.Mutex
	entries map[string]reverseClientCacheEntry
}

type reverseClientCacheEntry struct {
	credentialSignature [sha256.Size]byte
	client              *newapi.ReverseClient
}

func (s *Server) reverseClientForTarget(ctx context.Context, target config.AggregationTarget) (*newapi.ReverseClient, error) {
	cacheKey := strings.TrimSpace(target.Code)
	if cacheKey == "" {
		return nil, fmt.Errorf("aggregation target code is required")
	}
	signature := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimRight(strings.TrimSpace(target.BaseURL), "/"),
		strings.TrimSpace(target.ReverseUsername),
		target.ReversePassword,
	}, "\x00")))

	s.reverseClients.mu.Lock()

	if entry, ok := s.reverseClients.entries[cacheKey]; ok && entry.credentialSignature == signature {
		s.reverseClients.mu.Unlock()
		return entry.client, nil
	}
	client, err := newapi.NewReverseClient(target.BaseURL, target.ReverseUsername, target.ReversePassword)
	if err != nil {
		s.reverseClients.mu.Unlock()
		return nil, err
	}
	stale := s.reverseClients.entries[cacheKey].client
	if s.reverseClients.entries == nil {
		s.reverseClients.entries = make(map[string]reverseClientCacheEntry)
	}
	s.reverseClients.entries[cacheKey] = reverseClientCacheEntry{
		credentialSignature: signature,
		client:              client,
	}
	s.reverseClients.mu.Unlock()

	if stale != nil {
		logoutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_ = stale.Close(logoutCtx)
		cancel()
	}
	return client, nil
}

func (s *Server) Close(ctx context.Context) error {
	s.reverseClients.mu.Lock()
	entries := s.reverseClients.entries
	s.reverseClients.entries = nil
	s.reverseClients.mu.Unlock()

	closeErrors := make([]error, 0)
	for _, entry := range entries {
		if entry.client == nil {
			continue
		}
		if err := entry.client.Close(ctx); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}
