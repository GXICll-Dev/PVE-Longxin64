package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/operations"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

func TestOperationEventStreamReturnsTerminalSnapshot(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	operation := createHTTPTestOperation(t, repository)
	operation = completeHTTPTestOperation(t, repository, operation)
	handler := newEventTestHandler(repository, operations.NewEventLog(16, 8), 0, 0)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operation.ID+"/events", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("unexpected content type %q", contentType)
	}
	if response.Header().Get("Cache-Control") != "no-cache, no-transform" || response.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("missing streaming headers: %+v", response.Header())
	}
	frames := parseSSEFrames(t, response.Body.Bytes())
	if len(frames) != 1 || frames[0].event != string(operations.EventOperationSnapshot) {
		t.Fatalf("unexpected SSE frames: %+v\n%s", frames, response.Body.String())
	}
	var event operations.OperationEvent
	if err := json.Unmarshal(frames[0].data, &event); err != nil {
		t.Fatalf("decode SSE data: %v", err)
	}
	if event.Sequence != frames[0].id || event.OperationID != operation.ID || event.ItemID != "" {
		t.Fatalf("unexpected event identity: %+v", event)
	}
	if event.Timestamp.IsZero() || event.OperationStatus != domain.OperationSucceeded {
		t.Fatalf("unexpected terminal event: %+v", event)
	}
	if event.Progress.Completed != event.Progress.Total || len(event.Items) != len(operation.Items) {
		t.Fatalf("terminal snapshot does not contain full progress: %+v", event)
	}
}

func TestOperationEventStreamReplaysAfterLastEventID(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	operation := createHTTPTestOperation(t, repository)
	eventLog := operations.NewEventLog(64, 8)
	seed := eventLog.EventsSince(operation, nil)[0]
	operation = completeHTTPTestOperation(t, repository, operation)
	handler := newEventTestHandler(repository, eventLog, 0, 0)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operation.ID+"/events", nil)
	request.Header.Set("Last-Event-ID", strconv.FormatInt(seed.Sequence, 10))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	frames := parseSSEFrames(t, response.Body.Bytes())
	if len(frames) < 2 {
		t.Fatalf("expected parent and item deltas, got %d: %s", len(frames), response.Body.String())
	}
	previous := seed.Sequence
	foundItem := false
	for _, frame := range frames {
		if frame.id <= previous {
			t.Fatalf("non-monotonic event sequence: previous=%d current=%d", previous, frame.id)
		}
		previous = frame.id
		var event operations.OperationEvent
		if err := json.Unmarshal(frame.data, &event); err != nil {
			t.Fatalf("decode event %d: %v", frame.id, err)
		}
		if event.OperationStatus != domain.OperationSucceeded {
			t.Fatalf("replayed stale operation status: %+v", event)
		}
		if event.ItemID != "" {
			foundItem = true
		}
	}
	if !foundItem {
		t.Fatal("replay did not include any item-level update")
	}

	terminalReconnect := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operation.ID+"/events", nil)
	terminalReconnect.Header.Set("Last-Event-ID", strconv.FormatInt(previous, 10))
	terminalResponse := httptest.NewRecorder()
	handler.ServeHTTP(terminalResponse, terminalReconnect)
	if frames := parseSSEFrames(t, terminalResponse.Body.Bytes()); len(frames) != 0 {
		t.Fatalf("terminal reconnect duplicated events: %+v", frames)
	}

	future := previous + 100
	resetRequest := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operation.ID+"/events", nil)
	resetRequest.Header.Set("Last-Event-ID", strconv.FormatInt(future, 10))
	resetResponse := httptest.NewRecorder()
	handler.ServeHTTP(resetResponse, resetRequest)
	resetFrames := parseSSEFrames(t, resetResponse.Body.Bytes())
	if len(resetFrames) != 1 || resetFrames[0].id != future+1 {
		t.Fatalf("unexpected reset frame: %+v", resetFrames)
	}
	var resetEvent operations.OperationEvent
	if err := json.Unmarshal(resetFrames[0].data, &resetEvent); err != nil {
		t.Fatalf("decode reset event: %v", err)
	}
	if !resetEvent.Reset || resetEvent.EventType != operations.EventOperationSnapshot || len(resetEvent.Items) != len(operation.Items) {
		t.Fatalf("reset event cannot rebuild terminal state: %+v", resetEvent)
	}
}

func TestOperationEventStreamValidatesLastEventIDAndOperation(t *testing.T) {
	t.Parallel()
	handler, repository := newTestHandler()
	operation := createHTTPTestOperation(t, repository)

	invalid := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operation.ID+"/events", nil)
	invalid.Header.Set("Last-Event-ID", "not-a-sequence")
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid Last-Event-ID returned %d: %s", invalidResponse.Code, invalidResponse.Body.String())
	}
	var problem errorBody
	if err := json.Unmarshal(invalidResponse.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode invalid Last-Event-ID response: %v", err)
	}
	if problem.ErrorCode != "INVALID_LAST_EVENT_ID" {
		t.Fatalf("unexpected error: %+v", problem)
	}

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/operations/00000000-0000-0000-0000-000000000000/events", nil)
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusNotFound || !strings.HasPrefix(missingResponse.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("missing operation returned %d %q: %s", missingResponse.Code, missingResponse.Header().Get("Content-Type"), missingResponse.Body.String())
	}
}

func TestOperationEventStreamHeartbeatsAndStopsOnClientCancel(t *testing.T) {
	t.Parallel()
	repository := store.NewDevelopmentRepository(time.Now())
	operation := createHTTPTestOperation(t, repository)
	handler := newEventTestHandler(repository, operations.NewEventLog(16, 8), time.Hour, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operation.ID+"/events", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("non-terminal stream ended before cancellation")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after client cancellation")
	}
	if !strings.Contains(response.Body.String(), ": heartbeat ") {
		t.Fatalf("stream did not emit a heartbeat: %s", response.Body.String())
	}
}

type sseFrame struct {
	id    int64
	event string
	data  []byte
}

func parseSSEFrames(t *testing.T, body []byte) []sseFrame {
	t.Helper()
	blocks := bytes.Split(body, []byte("\n\n"))
	frames := make([]sseFrame, 0, len(blocks))
	for _, block := range blocks {
		if !bytes.Contains(block, []byte("data: ")) {
			continue
		}
		frame := sseFrame{}
		for _, line := range bytes.Split(block, []byte("\n")) {
			switch {
			case bytes.HasPrefix(line, []byte("id: ")):
				id, err := strconv.ParseInt(string(bytes.TrimPrefix(line, []byte("id: "))), 10, 64)
				if err != nil {
					t.Fatalf("parse SSE ID: %v", err)
				}
				frame.id = id
			case bytes.HasPrefix(line, []byte("event: ")):
				frame.event = string(bytes.TrimPrefix(line, []byte("event: ")))
			case bytes.HasPrefix(line, []byte("data: ")):
				frame.data = append([]byte(nil), bytes.TrimPrefix(line, []byte("data: "))...)
			}
		}
		frames = append(frames, frame)
	}
	return frames
}

func newEventTestHandler(repository store.Repository, eventLog *operations.EventLog, pollInterval, heartbeatInterval time.Duration) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := operations.NewManager(repository, nil)
	return NewHandler(Options{
		Repository:             repository,
		Operations:             manager,
		OperationEvents:        eventLog,
		Logger:                 logger,
		EventPollInterval:      pollInterval,
		EventHeartbeatInterval: heartbeatInterval,
	})
}

func createHTTPTestOperation(t *testing.T, repository store.Repository) domain.Operation {
	t.Helper()
	classrooms, err := repository.ListClassrooms(context.Background())
	if err != nil || len(classrooms) == 0 {
		t.Fatalf("list classrooms: %v", err)
	}
	manager := operations.NewManager(repository, nil)
	result, err := manager.Create(context.Background(), classrooms[0].ID, "sse-"+domain.NewID(), "request-sse", operations.CreateRequest{Type: domain.OperationStart})
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}
	return result.Operation
}

func completeHTTPTestOperation(t *testing.T, repository store.Repository, operation domain.Operation) domain.Operation {
	t.Helper()
	now := operation.UpdatedAt.Add(time.Second)
	if err := operation.Transition(domain.OperationValidating, now); err != nil {
		t.Fatalf("transition to validating: %v", err)
	}
	if err := repository.SaveOperation(context.Background(), &operation); err != nil {
		t.Fatalf("save validating operation: %v", err)
	}
	now = now.Add(time.Second)
	if err := operation.Transition(domain.OperationRunning, now); err != nil {
		t.Fatalf("transition to running: %v", err)
	}
	if err := repository.SaveOperation(context.Background(), &operation); err != nil {
		t.Fatalf("save running operation: %v", err)
	}
	now = now.Add(time.Second)
	for index := range operation.Items {
		operation.Items[index].Status = domain.ItemSucceeded
		operation.Items[index].UpdatedAt = now
		operation.Items[index].CompletedAt = &now
	}
	if err := operation.Transition(domain.OperationSucceeded, now); err != nil {
		t.Fatalf("transition to succeeded: %v", err)
	}
	if err := repository.SaveOperation(context.Background(), &operation); err != nil {
		t.Fatalf("save succeeded operation: %v", err)
	}
	return operation
}
