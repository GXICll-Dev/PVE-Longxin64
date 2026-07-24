package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/operations"
)

const (
	eventStreamRetry        = 2 * time.Second
	eventStreamWriteTimeout = 10 * time.Second
)

func (api *API) operationEventStream(writer http.ResponseWriter, request *http.Request) {
	lastEventID, err := parseLastEventID(request.Header.Get("Last-Event-ID"))
	if err != nil {
		api.writeError(writer, request, http.StatusBadRequest, "INVALID_LAST_EVENT_ID", "Last-Event-ID 必须是有效的非负整数事件序列。", nil)
		return
	}

	operation, err := api.repository.GetOperation(request.Context(), request.PathValue("id"))
	if err != nil {
		api.storeError(writer, request, err)
		return
	}
	events := api.operationEvents.EventsSince(operation, lastEventID)

	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)

	controller := http.NewResponseController(writer)
	// Refreshing the deadline per frame avoids the server-wide absolute write
	// timeout while still disconnecting clients that stop reading.
	_ = controller.SetWriteDeadline(time.Now().Add(eventStreamWriteTimeout))
	if _, err := fmt.Fprintf(writer, "retry: %d\n\n", eventStreamRetry.Milliseconds()); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		api.logEventStreamError(request, operation.ID, "flush stream prelude", err)
		return
	}

	cursor := int64(0)
	if lastEventID != nil {
		cursor = *lastEventID
	}
	if err := api.writeOperationEvents(writer, controller, events, &cursor); err != nil {
		api.logEventStreamError(request, operation.ID, "write operation events", err)
		return
	}
	if operation.Status.Terminal() {
		return
	}

	pollTicker := time.NewTicker(api.eventPollInterval)
	heartbeatTicker := time.NewTicker(api.eventHeartbeatInterval)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-request.Context().Done():
			return
		case <-pollTicker.C:
			operation, err = api.repository.GetOperation(request.Context(), operation.ID)
			if err != nil {
				if request.Context().Err() == nil {
					api.logEventStreamError(request, operation.ID, "poll operation", err)
				}
				return
			}
			after := cursor
			events = api.operationEvents.EventsSince(operation, &after)
			if err := api.writeOperationEvents(writer, controller, events, &cursor); err != nil {
				api.logEventStreamError(request, operation.ID, "write operation events", err)
				return
			}
			if operation.Status.Terminal() {
				return
			}
		case <-heartbeatTicker.C:
			_ = controller.SetWriteDeadline(time.Now().Add(eventStreamWriteTimeout))
			if _, err := fmt.Fprintf(writer, ": heartbeat %s\n\n", api.now().UTC().Format(time.RFC3339Nano)); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func parseLastEventID(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	sequence, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || sequence < 0 || sequence > operations.MaxLastEventID {
		return nil, fmt.Errorf("invalid Last-Event-ID")
	}
	return &sequence, nil
}

func (api *API) writeOperationEvents(writer http.ResponseWriter, controller *http.ResponseController, events []operations.OperationEvent, cursor *int64) error {
	for _, event := range events {
		_ = controller.SetWriteDeadline(time.Now().Add(eventStreamWriteTimeout))
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode event %d: %w", event.Sequence, err)
		}
		if _, err := fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.EventType, payload); err != nil {
			return err
		}
		if err := controller.Flush(); err != nil {
			return err
		}
		*cursor = event.Sequence
	}
	return nil
}

func (api *API) logEventStreamError(request *http.Request, operationID, action string, err error) {
	if request.Context().Err() != nil {
		return
	}
	api.logger.WarnContext(request.Context(), "operation event stream ended",
		"request_id", RequestID(request.Context()),
		"operation_id", operationID,
		"action", action,
		"error", err,
	)
}
