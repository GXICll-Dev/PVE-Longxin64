package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/operations"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

func newTestHandler() (http.Handler, *store.MemoryRepository) {
	repository := store.NewDevelopmentRepository(time.Now())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := operations.NewManager(repository, nil)
	return NewHandler(Options{Repository: repository, Operations: manager, Logger: logger}), repository
}

func TestReadRoutesAndSecurityHeaders(t *testing.T) {
	t.Parallel()
	handler, repository := newTestHandler()
	classrooms, _ := repository.ListClassrooms(context.Background())
	tests := []string{
		"/api/v1/health",
		"/api/v1/readiness",
		"/api/v1/dashboard",
		"/api/v1/classrooms",
		"/api/v1/classrooms/" + classrooms[0].ID,
		"/api/v1/operations",
	}
	for _, path := range tests {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d: %s", path, response.Code, response.Body.String())
		}
		if response.Header().Get("X-Request-ID") == "" {
			t.Fatalf("GET %s did not set X-Request-ID", path)
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("GET %s did not set security headers", path)
		}
	}
}

func TestHealthIncludesVersion(t *testing.T) {
	t.Parallel()
	handler, _ := newTestHandler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if health.Status != "ok" || health.Version == "" {
		t.Fatalf("unexpected health response: %+v", health)
	}
}

func TestCreateOperationRequiresIdempotencyAndReturnsSameOperation(t *testing.T) {
	t.Parallel()
	handler, repository := newTestHandler()
	classrooms, _ := repository.ListClassrooms(context.Background())
	path := "/api/v1/classrooms/" + classrooms[0].ID + "/operations"
	body := []byte(`{"type":"START"}`)

	missingKey := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	missingKey.Header.Set("Content-Type", "application/json")
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missingKey)
	if missingResponse.Code != http.StatusBadRequest {
		t.Fatalf("missing key returned %d: %s", missingResponse.Code, missingResponse.Body.String())
	}
	var problem errorBody
	if err := json.Unmarshal(missingResponse.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem response: %v", err)
	}
	if problem.ErrorCode != "IDEMPOTENCY_KEY_REQUIRED" || problem.RequestID == "" {
		t.Fatalf("unexpected problem response: %+v", problem)
	}

	firstID := ""
	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "classroom-start-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("attempt %d returned %d: %s", attempt, response.Code, response.Body.String())
		}
		var operation domain.Operation
		if err := json.Unmarshal(response.Body.Bytes(), &operation); err != nil {
			t.Fatalf("decode operation: %v", err)
		}
		if len(operation.Items) != classrooms[0].SeatsTotal {
			t.Fatalf("operation has %d items, expected %d", len(operation.Items), classrooms[0].SeatsTotal)
		}
		if firstID == "" {
			firstID = operation.ID
		} else if operation.ID != firstID {
			t.Fatalf("idempotent repeat returned %s, expected %s", operation.ID, firstID)
		}
		if response.Header().Get("Location") != "/api/v1/operations/"+operation.ID {
			t.Fatalf("unexpected Location header: %s", response.Header().Get("Location"))
		}
	}
}

func TestUnknownJSONFieldIsRejected(t *testing.T) {
	t.Parallel()
	handler, repository := newTestHandler()
	classrooms, _ := repository.ListClassrooms(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/classrooms/"+classrooms[0].ID+"/operations", bytes.NewBufferString(`{"type":"START","dangerous":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "unknown-field")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
}
