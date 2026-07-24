package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = domain.NewID()
		}
		writer.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(request.Context(), requestIDKey, requestID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return false
	}
	return true
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), usb=()")
		writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		if request.TLS != nil || request.Header.Get("X-Forwarded-Proto") == "https" {
			writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(writer, request)
	})
}

func corsMiddleware(origins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
				writer.Header().Set("Access-Control-Allow-Credentials", "true")
				writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, Last-Event-ID, X-Request-ID")
				writer.Header().Set("Access-Control-Max-Age", "600")
				writer.Header().Add("Vary", "Origin")
			} else if request.Method == http.MethodOptions {
				http.Error(writer, "origin is not allowed", http.StatusForbidden)
				return
			}
		}
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (recorder *responseRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

func (recorder *responseRecorder) WriteHeader(status int) {
	if recorder.status != 0 {
		return
	}
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *responseRecorder) Write(data []byte) (int, error) {
	if recorder.status == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	written, err := recorder.ResponseWriter.Write(data)
	recorder.bytes += written
	return written, err
}

func accessLogMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: writer}
		next.ServeHTTP(recorder, request)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.LogAttrs(request.Context(), slog.LevelInfo, "HTTP request",
			slog.String("request_id", RequestID(request.Context())),
			slog.String("method", request.Method),
			slog.String("path", request.URL.Path),
			slog.Int("status", status),
			slog.Int("response_bytes", recorder.bytes),
			slog.Duration("duration", time.Since(started)),
			slog.String("remote_ip", remoteIP(request.RemoteAddr)),
		)
	})
}

func recoveryMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(request.Context(), "panic recovered",
					"request_id", RequestID(request.Context()),
					"method", request.Method,
					"path", request.URL.Path,
					"panic_type", fmt.Sprintf("%T", recovered),
					"stack", string(debug.Stack()),
				)
				writeJSON(writer, http.StatusInternalServerError, errorBody{
					ErrorCode: "INTERNAL_ERROR",
					Message:   "服务暂时无法完成请求。",
					RequestID: RequestID(request.Context()),
				})
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

func remoteIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	return strings.TrimSpace(remoteAddress)
}
