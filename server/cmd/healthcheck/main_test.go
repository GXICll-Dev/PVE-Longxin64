package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeAcceptsSuccessfulResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := probe(context.Background(), server.Client(), server.URL); err != nil {
		t.Fatalf("probe successful endpoint: %v", err)
	}
}

func TestProbeRejectsUnreadyResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := probe(context.Background(), server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "503 Service Unavailable") {
		t.Fatalf("expected readiness failure, got %v", err)
	}
}

func TestRunRequiresExactlyOneURL(t *testing.T) {
	t.Parallel()
	if err := run(nil); err == nil {
		t.Fatal("run must reject a missing URL")
	}
}
