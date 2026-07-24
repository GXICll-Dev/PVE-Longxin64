package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/config"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/httpapi"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/operations"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

func TestHTTPServerShutdownCancelsActiveOperationEventStream(t *testing.T) {
	processContext, cancelProcess := context.WithCancel(context.Background())
	repository := store.NewDevelopmentRepository(time.Now())
	t.Cleanup(repository.Close)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := operations.NewManager(repository, nil)
	classrooms, err := repository.ListClassrooms(context.Background())
	if err != nil || len(classrooms) == 0 {
		t.Fatalf("list classrooms: %v", err)
	}
	created, err := manager.Create(
		context.Background(),
		classrooms[0].ID,
		"shutdown-sse-test",
		"request-shutdown-sse-test",
		operations.CreateRequest{Type: domain.OperationStart},
	)
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}
	handler := httpapi.NewHandler(httpapi.Options{
		Repository: repository,
		Operations: manager,
		Logger:     logger,
	})
	server := newHTTPServer(config.Config{
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       time.Second,
	}, handler, logger, processContext)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		cancelProcess()
		_ = server.Close()
	})

	client := &http.Client{Timeout: 3 * time.Second}
	request, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/api/v1/operations/"+created.Operation.ID+"/events", nil)
	if err != nil {
		t.Fatalf("create SSE request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("open SSE stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("unexpected SSE response: status=%d content_type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}

	// Production shutdown cancels this process context before calling Shutdown.
	// BaseContext must propagate that cancellation to the otherwise long-lived
	// non-terminal SSE handler, allowing Shutdown to finish instead of timing out.
	cancelProcess()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		t.Fatalf("shutdown with active SSE stream: %v", err)
	}
	select {
	case err := <-serveDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve() returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not return after shutdown")
	}
}
