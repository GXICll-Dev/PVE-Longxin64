package pve

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

const (
	testClusterID   = "70000000-0000-4000-8000-000000000001"
	testManagedPool = "cloud-classroom-managed"
	testTokenID     = "cloudclass@pve!controller"
	testTokenSecret = "test-token-secret-must-never-leak"
	testAuthHeader  = "PVEAPIToken=" + testTokenID + "=" + testTokenSecret
	testUPID        = "UPID:pve-a:00000001:00000002:00000003:qmstart:101:cloudclass@pve:"
)

func TestHTTPAdapterSubmitAndWait(t *testing.T) {
	var pollCount atomic.Int32
	var requestCount atomic.Int32
	server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		if request.Header.Get("Authorization") != testAuthHeader {
			t.Errorf("unexpected Authorization header")
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api2/json/nodes/pve-a/qemu/101/status/start":
			writeJSON(writer, http.StatusOK, `{"data":"`+testUPID+`"}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api2/json/nodes/pve-a/tasks/"+testUPID+"/status":
			if pollCount.Add(1) == 1 {
				writeJSON(writer, http.StatusOK, `{"data":{"status":"running"}}`)
				return
			}
			writeJSON(writer, http.StatusOK, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Errorf("unexpected PVE request: %s %s", request.Method, request.URL.String())
			writeJSON(writer, http.StatusNotFound, `{"data":null}`)
		}
	}))
	defer server.Close()

	adapter := newTestHTTPAdapter(t, server, nil)
	upid, err := adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if upid != testUPID {
		t.Fatalf("Submit() UPID = %q, want %q", upid, testUPID)
	}

	result, err := adapter.Wait(context.Background(), upid)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if !result.Succeeded || result.Code != "OK" {
		t.Fatalf("Wait() result = %+v", result)
	}
	if pollCount.Load() < 2 {
		t.Fatalf("task was polled %d times, want at least 2", pollCount.Load())
	}
	if requestCount.Load() < 3 {
		t.Fatalf("PVE received %d delegated requests, want at least 3", requestCount.Load())
	}
}

func TestHTTPAdapterPrecheckUsesRestartSafeSyntheticTask(t *testing.T) {
	server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != testAuthHeader {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api2/json/nodes/pve-a/qemu/101/status/current":
			writeJSON(writer, http.StatusOK, `{"data":{"status":"running"}}`)
		default:
			writeJSON(writer, http.StatusNotFound, `{"data":null}`)
		}
	}))
	defer server.Close()

	adapter := newTestHTTPAdapter(t, server, nil)
	taskID, err := adapter.Submit(context.Background(), Request{ClusterID: testClusterID, PVEVMID: 101, Type: domain.OperationPrecheck})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if !strings.HasPrefix(taskID, precheckTaskPrefix) {
		t.Fatalf("precheck task ID = %q", taskID)
	}
	result, err := adapter.Wait(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if !result.Succeeded {
		t.Fatalf("Wait() result = %+v", result)
	}
}

func TestHTTPAdapterNormalizesAuthenticationAndPermissionErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		expected ErrorCode
	}{
		{name: "authentication", status: http.StatusUnauthorized, expected: ErrorAuthentication},
		{name: "permission", status: http.StatusForbidden, expected: ErrorPermission},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(writer, test.status, `{"data":null}`)
			}))
			defer server.Close()
			adapter := newTestHTTPAdapter(t, server, nil)

			_, err := adapter.Submit(context.Background(), Request{
				ClusterID: testClusterID,
				PVENode:   "pve-a",
				PVEVMID:   101,
				Type:      domain.OperationStart,
			})
			adapterError := requireAdapterError(t, err, test.expected)
			if adapterError.HTTPStatus != test.status {
				t.Fatalf("HTTPStatus = %d, want %d", adapterError.HTTPStatus, test.status)
			}
			if OutcomeUncertain(err) {
				t.Fatalf("explicit HTTP %d rejection must have a certain outcome", test.status)
			}
			assertNoToken(t, err)
		})
	}
}

func TestHTTPAdapterNormalizesPVEJSONErrorWithoutLeakingToken(t *testing.T) {
	server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusBadRequest, `{"data":null,"errors":{"vmid":"bad `+testTokenID+` `+testTokenSecret+`"}}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	_, err := adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVENode:   "pve-a",
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	adapterError := requireAdapterError(t, err, ErrorInvalid)
	if adapterError.Retryable {
		t.Fatal("PVE validation errors must not be retryable")
	}
	if OutcomeUncertain(err) {
		t.Fatal("PVE validation rejection must have a certain outcome")
	}
	assertNoToken(t, err)
}

func TestHTTPAdapterSubmitTimeoutIsOutcomeUncertain(t *testing.T) {
	started := make(chan struct{})
	server := newManagedTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, func(config *HTTPConfig) {
		config.RequestTimeout = 25 * time.Millisecond
	})

	_, err := adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVENode:   "pve-a",
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	<-started
	adapterError := requireAdapterError(t, err, ErrorTimeout)
	if adapterError.Retryable {
		t.Fatal("a timed-out mutating submission must not be blindly retried")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error does not wrap context deadline: %v", err)
	}
	if !OutcomeUncertain(err) {
		t.Fatal("timed-out mutating submission must have an uncertain outcome")
	}
	assertNoToken(t, err)
}

func TestHTTPAdapterSubmitCancellation(t *testing.T) {
	started := make(chan struct{})
	server := newManagedTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := adapter.Submit(ctx, Request{
			ClusterID: testClusterID,
			PVENode:   "pve-a",
			PVEVMID:   101,
			Type:      domain.OperationStart,
		})
		result <- err
	}()
	<-started
	cancel()

	select {
	case err := <-result:
		requireAdapterError(t, err, ErrorCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error does not wrap context cancellation: %v", err)
		}
		if !OutcomeUncertain(err) {
			t.Fatal("an in-flight cancelled submission must have an uncertain outcome")
		}
		assertNoToken(t, err)
	case <-time.After(time.Second):
		t.Fatal("Submit() did not honor context cancellation")
	}
}

func TestHTTPAdapterCancelledBeforeSubmitIsCertain(t *testing.T) {
	server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		t.Error("cancelled request reached PVE")
		writeJSON(writer, http.StatusOK, `{"data":"`+testUPID+`"}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := adapter.Submit(ctx, Request{ClusterID: testClusterID, PVENode: "pve-a", PVEVMID: 101, Type: domain.OperationStart})
	requireAdapterError(t, err, ErrorCancelled)
	if OutcomeUncertain(err) {
		t.Fatal("a request cancelled before dispatch has a certain outcome")
	}
}

func TestHTTPAdapterMissingOrInvalidUPIDIsOutcomeUncertain(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"data":null}`},
		{name: "invalid", body: `{"data":"not-a-pve-task"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(writer, http.StatusOK, test.body)
			}))
			defer server.Close()
			adapter := newTestHTTPAdapter(t, server, nil)

			_, err := adapter.Submit(context.Background(), Request{
				ClusterID: testClusterID,
				PVENode:   "pve-a",
				PVEVMID:   101,
				Type:      domain.OperationStart,
			})
			adapterError := requireAdapterError(t, err, ErrorOutcomeUncertain)
			if adapterError.Retryable {
				t.Fatal("missing UPID must not be blindly retried")
			}
			if !OutcomeUncertain(err) {
				t.Fatal("missing UPID must have an uncertain outcome")
			}
			assertNoToken(t, err)
		})
	}
}

func TestHTTPAdapterInterruptedSubmitResponseIsOutcomeUncertain(t *testing.T) {
	server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Error("TLS test server does not support hijacking")
			return
		}
		connection, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack() error = %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprint(buffered, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 200\r\nConnection: close\r\n\r\n{\"data\":\"UPID:pve-a")
		_ = buffered.Flush()
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	_, err := adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVENode:   "pve-a",
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	adapterError := requireAdapterError(t, err, ErrorOutcomeUncertain)
	if adapterError.Retryable || !OutcomeUncertain(err) {
		t.Fatalf("interrupted response classification = %+v", adapterError)
	}
	assertNoToken(t, err)
}

func TestHTTPAdapterTaskFailureIsExplicitAndSanitized(t *testing.T) {
	server := newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api2/json/nodes/pve-a/tasks/"+testUPID+"/status" {
			writeJSON(writer, http.StatusNotFound, `{"data":null}`)
			return
		}
		writeJSON(writer, http.StatusOK, `{"data":{"status":"stopped","exitstatus":"TASK ERROR: `+testTokenSecret+`"}}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	result, err := adapter.Wait(context.Background(), testUPID)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.Succeeded || result.Code != string(ErrorTaskFailed) {
		t.Fatalf("Wait() result = %+v", result)
	}
	if strings.Contains(result.Message, testTokenSecret) || strings.Contains(result.Message, testTokenID) {
		t.Fatalf("task result leaked credentials: %q", result.Message)
	}
}

func TestHTTPAdapterUnknownTask(t *testing.T) {
	server := newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusNotFound, `{"data":null}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	_, err := adapter.Wait(context.Background(), testUPID)
	adapterError := requireAdapterError(t, err, ErrorUnknownTask)
	if adapterError.HTTPStatus != http.StatusNotFound {
		t.Fatalf("HTTPStatus = %d", adapterError.HTTPStatus)
	}
	if !OutcomeUncertain(err) {
		t.Fatal("an unknown PVE task has an uncertain final outcome")
	}
}

func TestHTTPAdapterWaitHonorsTaskTimeout(t *testing.T) {
	server := newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, `{"data":{"status":"running"}}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, func(config *HTTPConfig) {
		config.TaskTimeout = 30 * time.Millisecond
		config.TaskPollInterval = 2 * time.Millisecond
	})

	_, err := adapter.Wait(context.Background(), testUPID)
	requireAdapterError(t, err, ErrorTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error does not wrap task deadline: %v", err)
	}
	if !OutcomeUncertain(err) {
		t.Fatal("task polling timeout must leave the final outcome uncertain")
	}
}

func TestHTTPAdapterStrictTLSByDefault(t *testing.T) {
	var reached atomic.Bool
	server := newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		writeJSON(writer, http.StatusOK, `{"data":"`+testUPID+`"}`)
	}))
	defer server.Close()
	adapter, err := NewHTTPAdapter(HTTPConfig{
		BaseURL:        server.URL,
		ClusterID:      testClusterID,
		ManagedPool:    testManagedPool,
		TokenID:        testTokenID,
		TokenSecret:    testTokenSecret,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter() error = %v", err)
	}
	defer adapter.CloseIdleConnections()

	_, err = adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVENode:   "pve-a",
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	requireAdapterError(t, err, ErrorTLS)
	if reached.Load() {
		t.Fatal("HTTP request reached a server whose certificate was not trusted")
	}
	if OutcomeUncertain(err) {
		t.Fatal("TLS verification failed before PVE could accept the request")
	}
	assertNoToken(t, err)
}

func TestHTTPAdapterRestoreRequiresSnapshotName(t *testing.T) {
	server := newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		t.Error("invalid restore request reached PVE")
		writeJSON(writer, http.StatusOK, `{"data":"`+testUPID+`"}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	_, err := adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVENode:   "pve-a",
		PVEVMID:   101,
		Type:      domain.OperationRestore,
	})
	requireAdapterError(t, err, ErrorInvalid)
	if OutcomeUncertain(err) {
		t.Fatal("locally rejected restore request has a certain outcome")
	}
}

func TestHTTPAdapterRejectsMismatchedClusterBeforeCallingPVE(t *testing.T) {
	server := newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		t.Error("out-of-scope cluster request reached PVE")
		writeJSON(writer, http.StatusOK, `{"data":null}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	_, err := adapter.Submit(context.Background(), Request{
		ClusterID: "another-cluster",
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	requireAdapterError(t, err, ErrorOutOfScope)
	if OutcomeUncertain(err) {
		t.Fatal("locally rejected cluster mismatch has a certain outcome")
	}
}

func TestHTTPAdapterRejectsVMOutsideManagedPool(t *testing.T) {
	server := newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api2/json/pools/"+testManagedPool {
			t.Errorf("unexpected PVE request: %s %s", request.Method, request.URL.Path)
			writeJSON(writer, http.StatusNotFound, `{"data":null}`)
			return
		}
		writeJSON(writer, http.StatusOK, `{"data":{"members":[]}}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	_, err := adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	requireAdapterError(t, err, ErrorOutOfScope)
	if OutcomeUncertain(err) {
		t.Fatal("managed pool rejection happens before mutation")
	}
}

func TestHTTPAdapterRejectsNodeThatDisagreesWithManagedPool(t *testing.T) {
	server := newManagedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Errorf("node mismatch reached a mutating endpoint: %s %s", request.Method, request.URL.Path)
		writeJSON(writer, http.StatusOK, `{"data":"`+testUPID+`"}`)
	}))
	defer server.Close()
	adapter := newTestHTTPAdapter(t, server, nil)

	_, err := adapter.Submit(context.Background(), Request{
		ClusterID: testClusterID,
		PVENode:   "pve-b",
		PVEVMID:   101,
		Type:      domain.OperationStart,
	})
	requireAdapterError(t, err, ErrorOutOfScope)
	if OutcomeUncertain(err) {
		t.Fatal("managed pool node mismatch happens before mutation")
	}
}

func TestNewHTTPAdapterRejectsInsecureOrInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config HTTPConfig
	}{
		{
			name: "plain HTTP",
			config: HTTPConfig{
				BaseURL:     "http://pve.example.test:8006",
				ClusterID:   testClusterID,
				ManagedPool: testManagedPool,
				TokenID:     testTokenID,
				TokenSecret: testTokenSecret,
			},
		},
		{
			name: "invalid CA",
			config: HTTPConfig{
				BaseURL:          "https://pve.example.test:8006",
				ClusterID:        testClusterID,
				ManagedPool:      testManagedPool,
				TokenID:          testTokenID,
				TokenSecret:      testTokenSecret,
				CACertificatePEM: []byte("not a certificate"),
			},
		},
		{
			name: "missing cluster ID",
			config: HTTPConfig{
				BaseURL:     "https://pve.example.test:8006",
				ManagedPool: testManagedPool,
				TokenID:     testTokenID,
				TokenSecret: testTokenSecret,
			},
		},
		{
			name: "missing managed pool",
			config: HTTPConfig{
				BaseURL:     "https://pve.example.test:8006",
				ClusterID:   testClusterID,
				TokenID:     testTokenID,
				TokenSecret: testTokenSecret,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewHTTPAdapter(test.config); err == nil {
				t.Fatal("NewHTTPAdapter() succeeded for invalid configuration")
			}
		})
	}
}

func newTestHTTPAdapter(t *testing.T, server *httptest.Server, mutate func(*HTTPConfig)) *HTTPAdapter {
	t.Helper()
	certificate := server.Certificate()
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	config := HTTPConfig{
		BaseURL:          server.URL,
		ClusterID:        testClusterID,
		ManagedPool:      testManagedPool,
		TokenID:          testTokenID,
		TokenSecret:      testTokenSecret,
		CACertificatePEM: caPEM,
		RequestTimeout:   time.Second,
		TaskTimeout:      time.Second,
		TaskPollInterval: time.Millisecond,
	}
	if mutate != nil {
		mutate(&config)
	}
	adapter, err := NewHTTPAdapter(config)
	if err != nil {
		t.Fatalf("NewHTTPAdapter() error = %v", err)
	}
	t.Cleanup(adapter.CloseIdleConnections)
	return adapter
}

func newManagedTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	return newQuietTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/api2/json/pools/"+testManagedPool {
			if request.Header.Get("Authorization") != testAuthHeader {
				t.Errorf("managed pool request used an unexpected Authorization header")
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeJSON(writer, http.StatusOK, `{"data":{"members":[{"node":"pve-a","type":"qemu","vmid":101}]}}`)
			return
		}
		handler.ServeHTTP(writer, request)
	}))
}

func newQuietTLSServer(handler http.Handler) *httptest.Server {
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	return server
}

func writeJSON(writer http.ResponseWriter, status int, body string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, body)
}

func requireAdapterError(t *testing.T, err error, code ErrorCode) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	var adapterError *Error
	if !errors.As(err, &adapterError) {
		t.Fatalf("error type = %T, want *pve.Error: %v", err, err)
	}
	if adapterError.Code != code {
		t.Fatalf("error code = %s, want %s: %v", adapterError.Code, code, err)
	}
	return adapterError
}

func assertNoToken(t *testing.T, value any) {
	t.Helper()
	formatted := fmt.Sprintf("%+v", value)
	if strings.Contains(formatted, testTokenID) || strings.Contains(formatted, testTokenSecret) || strings.Contains(formatted, testAuthHeader) {
		t.Fatalf("value leaked PVE token material: %s", formatted)
	}
}
