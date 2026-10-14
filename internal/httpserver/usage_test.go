package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"keyhub/internal/config"
	"keyhub/internal/tokensupply"
)

func TestAggregationTargetUsageErrorRedactsTargetDetails(t *testing.T) {
	target := config.AggregationTarget{
		Code:            "qiniu",
		Name:            "七牛平台",
		BaseURL:         "https://z.tokensupply.cc",
		Token:           "target-token",
		ReverseUsername: "provider-user",
		ReversePassword: "provider-pass",
	}
	err := aggregationTargetUsageError(
		"tokensupply 用量同步失败",
		target,
		fmt.Errorf(`call tokensupply endpoint: Get "https://z.tokensupply.cc/api/user/login": lookup z.tokensupply.cc failed for qiniu 七牛平台 target-token provider-user provider-pass`),
	)

	message := err.Error()
	for _, leaked := range []string{
		"qiniu",
		"七牛平台",
		"https://z.tokensupply.cc",
		"z.tokensupply.cc",
		"target-token",
		"provider-user",
		"provider-pass",
	} {
		if strings.Contains(message, leaked) {
			t.Fatalf("error message leaked %q: %s", leaked, message)
		}
	}
	if !strings.Contains(message, "tokensupply 用量同步失败") {
		t.Fatalf("error message = %q, want sync failure context", message)
	}
	if !strings.Contains(message, "[已隐藏]") {
		t.Fatalf("error message = %q, want redaction placeholder", message)
	}
}

func TestLoadSingleAggregationTargetUsageRedactsTokenSupplyError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway for qiniu via " + r.Host))
	}))
	defer upstream.Close()

	server := &Server{config: config.Config{NewAPIQuotaPerUSD: 500000}}
	_, err := server.loadSingleAggregationTargetUsage(context.Background(), config.AggregationTarget{
		Code:            "qiniu",
		Name:            "七牛平台",
		BaseURL:         upstream.URL,
		ConnectionMode:  aggregationTargetConnectionModeTokenSupply,
		ReverseUsername: "provider-user",
		ReversePassword: "provider-pass",
	})
	if err == nil {
		t.Fatal("loadSingleAggregationTargetUsage error = nil, want upstream failure")
	}

	message := err.Error()
	parsed, parseErr := url.Parse(upstream.URL)
	if parseErr != nil {
		t.Fatalf("parse upstream URL: %v", parseErr)
	}
	for _, leaked := range []string{
		"qiniu",
		"七牛平台",
		upstream.URL,
		parsed.Host,
		parsed.Hostname(),
		"provider-user",
		"provider-pass",
	} {
		if strings.Contains(message, leaked) {
			t.Fatalf("error message leaked %q: %s", leaked, message)
		}
	}
	if !strings.Contains(message, "tokensupply 用量同步失败") {
		t.Fatalf("error message = %q, want tokensupply context", message)
	}
	if !strings.Contains(message, "tokensupply http status 502") {
		t.Fatalf("error message = %q, want upstream status", message)
	}
}

func TestLoadAggregationTargetUsageContinuesAfterTargetFailure(t *testing.T) {
	failingUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway for bad-target via " + r.Host))
	}))
	defer failingUpstream.Close()

	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"username":"healthy-user","purchase_provider":"demo"}}`))
		case "/api/apikey/list":
			if got := r.Header.Get("x-provider-user"); got != "healthy-user" {
				t.Fatalf("x-provider-user = %q, want healthy-user", got)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"42","actual_cost":1.5}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer healthyUpstream.Close()

	server := &Server{config: config.Config{NewAPIQuotaPerUSD: 500000}}
	usageByTarget, failedTargetCodes, failures, err := server.loadAggregationTargetUsage(context.Background(), []config.AggregationTarget{
		{
			Code:            "bad-target",
			Name:            "Broken Platform",
			BaseURL:         failingUpstream.URL,
			ConnectionMode:  aggregationTargetConnectionModeTokenSupply,
			ReverseUsername: "bad-user",
			ReversePassword: "bad-pass",
		},
		{
			Code:            "healthy-target",
			Name:            "Healthy Platform",
			BaseURL:         healthyUpstream.URL,
			ConnectionMode:  aggregationTargetConnectionModeTokenSupply,
			ReverseUsername: "healthy-user",
			ReversePassword: "healthy-pass",
		},
	})
	if err != nil {
		t.Fatalf("loadAggregationTargetUsage() error = %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures len = %d, want 1", len(failures))
	}
	if _, ok := failedTargetCodes["bad-target"]; !ok {
		t.Fatalf("failedTargetCodes = %#v, want bad-target", failedTargetCodes)
	}
	healthyChannelID := tokensupply.RemoteIDNumber("42")
	if got := usageByTarget["healthy-target"][healthyChannelID].UsedQuota; got != 750000 {
		t.Fatalf("healthy usage = %d, want 750000", got)
	}

	failureMessage := failures[0].Error
	failingURL, parseErr := url.Parse(failingUpstream.URL)
	if parseErr != nil {
		t.Fatalf("parse failing upstream URL: %v", parseErr)
	}
	for _, leaked := range []string{
		"bad-target",
		"Broken Platform",
		failingUpstream.URL,
		failingURL.Host,
		failingURL.Hostname(),
		"bad-user",
		"bad-pass",
	} {
		if strings.Contains(failureMessage, leaked) {
			t.Fatalf("failure message leaked %q: %s", leaked, failureMessage)
		}
	}
	if !strings.Contains(failureMessage, "tokensupply http status 502") {
		t.Fatalf("failure message = %q, want upstream status", failureMessage)
	}
}

func TestLoadSingleAggregationTargetUsageReusesReverseSession(t *testing.T) {
	var loginCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			loginCount.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "cached", Path: "/"})
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":12,"role":10}}`))
		case "/api/channel/":
			if _, err := r.Cookie("session"); err != nil {
				t.Fatalf("session cookie error = %v", err)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":44,"used_quota":12345,"status":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := &Server{config: config.Config{NewAPIQuotaPerUSD: 500000}}
	target := config.AggregationTarget{
		Code:            "reverse-target",
		BaseURL:         upstream.URL,
		ConnectionMode:  aggregationTargetConnectionModeNewAPIReverse,
		ReverseUsername: "admin",
		ReversePassword: "secret",
	}
	for attempt := 0; attempt < 2; attempt++ {
		items, err := server.loadSingleAggregationTargetUsage(context.Background(), target)
		if err != nil {
			t.Fatalf("loadSingleAggregationTargetUsage() attempt %d error = %v", attempt+1, err)
		}
		if len(items) != 1 || items[0].ChannelID != 44 || items[0].UsedQuota != 12345 || items[0].Status != 3 {
			t.Fatalf("items = %+v", items)
		}
	}
	if got := loginCount.Load(); got != 1 {
		t.Fatalf("login count = %d, want 1", got)
	}
}
