package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/buildinfo"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/operations"
	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/store"
)

type Options struct {
	Repository             store.Repository
	Operations             *operations.Manager
	OperationEvents        *operations.EventLog
	Logger                 *slog.Logger
	AllowedOrigins         []string
	EventPollInterval      time.Duration
	EventHeartbeatInterval time.Duration
}

type API struct {
	repository             store.Repository
	operations             *operations.Manager
	operationEvents        *operations.EventLog
	logger                 *slog.Logger
	eventPollInterval      time.Duration
	eventHeartbeatInterval time.Duration
	now                    func() time.Time
}

func NewHandler(options Options) http.Handler {
	if options.OperationEvents == nil {
		options.OperationEvents = operations.NewEventLog(0, 0)
	}
	if options.EventPollInterval <= 0 {
		options.EventPollInterval = 500 * time.Millisecond
	}
	if options.EventHeartbeatInterval <= 0 {
		options.EventHeartbeatInterval = 15 * time.Second
	}
	api := &API{
		repository:             options.Repository,
		operations:             options.Operations,
		operationEvents:        options.OperationEvents,
		logger:                 options.Logger,
		eventPollInterval:      options.EventPollInterval,
		eventHeartbeatInterval: options.EventHeartbeatInterval,
		now:                    time.Now,
	}
	router := http.NewServeMux()
	router.HandleFunc("GET /api/v1/health", api.health)
	router.HandleFunc("GET /api/v1/readiness", api.readiness)
	router.HandleFunc("GET /api/v1/dashboard", api.dashboard)
	router.HandleFunc("GET /api/v1/classrooms", api.classrooms)
	router.HandleFunc("GET /api/v1/classrooms/{id}", api.classroom)
	router.HandleFunc("POST /api/v1/classrooms/{id}/operations", api.createOperation)
	router.HandleFunc("GET /api/v1/operations", api.operationsList)
	router.HandleFunc("GET /api/v1/operations/{id}/events", api.operationEventStream)
	router.HandleFunc("GET /api/v1/operations/{id}", api.operation)
	router.HandleFunc("GET /healthz", api.health)
	router.HandleFunc("GET /readyz", api.readiness)
	router.HandleFunc("/", api.notFound)

	handler := recoveryMiddleware(api.logger, router)
	handler = accessLogMiddleware(api.logger, handler)
	handler = corsMiddleware(options.AllowedOrigins, handler)
	handler = securityHeadersMiddleware(handler)
	handler = requestIDMiddleware(handler)
	return handler
}

func (api *API) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "pve-classroom-api",
		"version": buildinfo.Version,
		"time":    api.now().UTC(),
	})
}

func (api *API) readiness(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := api.repository.Ping(ctx); err != nil {
		api.logger.WarnContext(request.Context(), "readiness check failed", "request_id", RequestID(request.Context()), "error", err)
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"checks": map[string]string{"store": "unavailable"},
			"time":   api.now().UTC(),
		})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"status": "ready",
		"checks": map[string]string{"store": "ok"},
		"time":   api.now().UTC(),
	})
}

func (api *API) dashboard(writer http.ResponseWriter, request *http.Request) {
	classrooms, err := api.repository.ListClassrooms(request.Context())
	if err != nil {
		api.internalError(writer, request, err)
		return
	}
	operationList, _, err := api.repository.ListOperations(request.Context(), 200)
	if err != nil {
		api.internalError(writer, request, err)
		return
	}
	dashboard := domain.Dashboard{GeneratedAt: api.now().UTC(), Alerts: make([]domain.Alert, 0)}
	dashboard.Summary.ClassroomsTotal = len(classrooms)
	for _, classroom := range classrooms {
		dashboard.Summary.SeatsTotal += classroom.SeatsTotal
		dashboard.Summary.SeatsReady += classroom.SeatsReady
		dashboard.Summary.ThinClientsOnline += classroom.ThinClientsOnline
		dashboard.Summary.DesktopsRunning += classroom.DesktopsRunning
		switch classroom.Status {
		case domain.ClassroomReady:
			dashboard.Summary.ClassroomsReady++
		case domain.ClassroomActive:
			dashboard.Summary.ClassroomsActive++
		case domain.ClassroomDegraded, domain.ClassroomOffline:
			dashboard.Alerts = append(dashboard.Alerts, domain.Alert{
				ID:          "classroom:" + classroom.ID,
				Severity:    "warning",
				Title:       classroom.Name + "需要关注",
				Description: "教室当前未达到完整就绪状态，请查看异常座位。",
				ResourceID:  classroom.ID,
			})
		}
	}
	for _, operation := range operationList {
		if !operation.Status.Terminal() && operation.Status != domain.OperationCancelRequested {
			dashboard.Summary.OperationsRunning++
		}
		if operation.Status == domain.OperationFailed || operation.Status == domain.OperationPartiallySucceeded {
			dashboard.Summary.OperationsFailed++
		}
	}
	writeJSON(writer, http.StatusOK, dashboard)
}

func (api *API) classrooms(writer http.ResponseWriter, request *http.Request) {
	items, err := api.repository.ListClassrooms(request.Context())
	if err != nil {
		api.internalError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (api *API) classroom(writer http.ResponseWriter, request *http.Request) {
	classroom, err := api.repository.GetClassroom(request.Context(), request.PathValue("id"))
	if err != nil {
		api.storeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, classroom)
}

func (api *API) createOperation(writer http.ResponseWriter, request *http.Request) {
	if err := requireJSONContentType(request); err != nil {
		api.writeError(writer, request, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", err.Error(), nil)
		return
	}
	var createRequest operations.CreateRequest
	if err := decodeJSON(writer, request, &createRequest); err != nil {
		api.writeError(writer, request, http.StatusBadRequest, "INVALID_JSON", "请求体不是有效的 JSON。", map[string]any{"reason": err.Error()})
		return
	}
	result, err := api.operations.Create(
		request.Context(),
		request.PathValue("id"),
		request.Header.Get("Idempotency-Key"),
		RequestID(request.Context()),
		createRequest,
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			api.writeError(writer, request, http.StatusNotFound, "CLASSROOM_NOT_FOUND", "未找到指定云教室。", nil)
		case errors.Is(err, store.ErrIdempotencyConflict):
			api.writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "该 Idempotency-Key 已用于不同的请求。", nil)
		case errors.Is(err, operations.ErrIdempotencyKeyRequired):
			api.writeError(writer, request, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "写请求必须提供 Idempotency-Key。", nil)
		case errors.Is(err, operations.ErrConfirmationRequired):
			api.writeError(writer, request, http.StatusUnprocessableEntity, "RESTORE_CONFIRMATION_REQUIRED", "还原操作必须填写原因并设置 confirmed=true。", nil)
		case errors.Is(err, operations.ErrInvalidRequest):
			api.writeError(writer, request, http.StatusBadRequest, "INVALID_OPERATION", err.Error(), nil)
		default:
			api.internalError(writer, request, err)
		}
		return
	}
	writer.Header().Set("Location", "/api/v1/operations/"+result.Operation.ID)
	writer.Header().Set("Retry-After", "1")
	writeJSON(writer, http.StatusAccepted, result.Operation)
}

func (api *API) operationsList(writer http.ResponseWriter, request *http.Request) {
	limit := 50
	if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 200 {
			api.writeError(writer, request, http.StatusBadRequest, "INVALID_LIMIT", "limit 必须是 1 到 200 之间的整数。", nil)
			return
		}
		limit = parsed
	}
	items, total, err := api.repository.ListOperations(request.Context(), limit)
	if err != nil {
		api.internalError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items, "total": total, "generated_at": api.now().UTC()})
}

func (api *API) operation(writer http.ResponseWriter, request *http.Request) {
	operation, err := api.repository.GetOperation(request.Context(), request.PathValue("id"))
	if err != nil {
		api.storeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, operation)
}

func (api *API) notFound(writer http.ResponseWriter, request *http.Request) {
	api.writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "未找到请求的 API 资源。", nil)
}

func (api *API) storeError(writer http.ResponseWriter, request *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		api.writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "未找到请求的资源。", nil)
		return
	}
	api.internalError(writer, request, err)
}

func (api *API) internalError(writer http.ResponseWriter, request *http.Request, err error) {
	api.logger.ErrorContext(request.Context(), "API request failed",
		"request_id", RequestID(request.Context()),
		"method", request.Method,
		"path", request.URL.Path,
		"error", err,
	)
	api.writeError(writer, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时无法完成请求。", nil)
}

type errorBody struct {
	ErrorCode string         `json:"error_code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}

func (api *API) writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string, details map[string]any) {
	writeJSON(writer, status, errorBody{
		ErrorCode: code,
		Message:   message,
		RequestID: RequestID(request.Context()),
		Details:   details,
	})
}

func requireJSONContentType(request *http.Request) error {
	contentType := request.Header.Get("Content-Type")
	if contentType == "" {
		return fmt.Errorf("Content-Type must be application/json")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("Content-Type must be application/json")
	}
	return nil
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		// Headers are already committed; the access log and request ID allow the
		// transport failure to be correlated without exposing internals.
		return
	}
}
