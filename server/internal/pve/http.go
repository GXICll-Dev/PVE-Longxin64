package pve

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
)

const (
	defaultRequestTimeout = 15 * time.Second
	defaultTaskTimeout    = 30 * time.Minute
	defaultPollInterval   = time.Second
	maxResponseBody       = 1 << 20
	precheckTaskPrefix    = "SYNC:PVE-PRECHECK:"
)

// HTTPConfig configures a production PVE HTTPS adapter.
//
// BaseURL may be either the PVE origin (for example https://pve:8006) or an
// endpoint ending in /api2/json. ClusterID binds platform records to this
// adapter, while ManagedPool is the only PVE pool the adapter may operate on.
// TokenID is the complete PVE token identifier, for example
// cloudclass@pve!controller. CACertificatePEM optionally extends the system
// trust store for an internal CA; TLS verification is never disabled.
type HTTPConfig struct {
	BaseURL          string
	ClusterID        string
	ManagedPool      string
	TokenID          string
	TokenSecret      string
	CACertificatePEM []byte
	RequestTimeout   time.Duration
	TaskTimeout      time.Duration
	TaskPollInterval time.Duration
}

// HTTPAdapter executes supported VM lifecycle operations using the PVE REST
// API and follows asynchronous tasks by their PVE UPID.
type HTTPAdapter struct {
	baseURL        *url.URL
	clusterID      string
	managedPool    string
	tokenID        string
	tokenSecret    string
	client         *http.Client
	requestTimeout time.Duration
	taskTimeout    time.Duration
	pollInterval   time.Duration
}

type requestPurpose uint8

const (
	purposePreflight requestPurpose = iota
	purposeMutation
	purposeTaskPoll
)

type apiEnvelope struct {
	Data json.RawMessage `json:"data"`
}

type managedPool struct {
	Members []managedPoolMember `json:"members"`
}

type managedPoolMember struct {
	Node string `json:"node"`
	Type string `json:"type"`
	VMID int    `json:"vmid"`
}

type taskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

type vmStatus struct {
	Status string `json:"status"`
}

func NewHTTPAdapter(config HTTPConfig) (*HTTPAdapter, error) {
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	if err := validateTokenPart("token ID", config.TokenID); err != nil {
		return nil, err
	}
	if err := validateTokenPart("token secret", config.TokenSecret); err != nil {
		return nil, err
	}
	clusterID := strings.TrimSpace(config.ClusterID)
	if !validIdentifier(clusterID) {
		return nil, errors.New("PVE cluster ID is required and must be a valid identifier")
	}
	managedPool := strings.TrimSpace(config.ManagedPool)
	if !validIdentifier(managedPool) {
		return nil, errors.New("PVE managed pool is required and must be a valid identifier")
	}
	if config.RequestTimeout < 0 || config.TaskTimeout < 0 || config.TaskPollInterval < 0 {
		return nil, errors.New("PVE adapter durations cannot be negative")
	}

	requestTimeout := config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = defaultRequestTimeout
	}
	taskTimeout := config.TaskTimeout
	if taskTimeout == 0 {
		taskTimeout = defaultTaskTimeout
	}
	pollInterval := config.TaskPollInterval
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if transport.TLSClientConfig != nil {
		tlsConfig = transport.TLSClientConfig.Clone()
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	if len(config.CACertificatePEM) > 0 {
		roots, poolErr := x509.SystemCertPool()
		if poolErr != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(config.CACertificatePEM) {
			return nil, errors.New("PVE CA certificate PEM contains no valid certificates")
		}
		tlsConfig.RootCAs = roots
	}
	// InsecureSkipVerify intentionally has no configuration path. PVE must use a
	// publicly trusted certificate or an explicitly supplied internal CA.
	tlsConfig.InsecureSkipVerify = false
	transport.TLSClientConfig = tlsConfig

	return &HTTPAdapter{
		baseURL:     baseURL,
		clusterID:   clusterID,
		managedPool: managedPool,
		tokenID:     config.TokenID,
		tokenSecret: config.TokenSecret,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		requestTimeout: requestTimeout,
		taskTimeout:    taskTimeout,
		pollInterval:   pollInterval,
	}, nil
}

// CloseIdleConnections releases idle PVE connections. Active requests are not
// interrupted.
func (adapter *HTTPAdapter) CloseIdleConnections() {
	adapter.client.CloseIdleConnections()
}

func (adapter *HTTPAdapter) Submit(ctx context.Context, request Request) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", contextError(err, true, false)
	}
	if !request.Type.Valid() {
		return "", newCertainError(ErrorInvalid, false, "不支持的 PVE 操作类型", nil, 0)
	}
	if request.PVEVMID <= 0 {
		return "", newCertainError(ErrorInvalid, false, "PVE VMID 必须为正整数", nil, 0)
	}
	if strings.TrimSpace(request.ClusterID) != adapter.clusterID {
		return "", newCertainError(ErrorOutOfScope, false, "目标资源不属于当前配置的 PVE 集群", nil, 0)
	}
	if request.Type == domain.OperationRestore {
		snapshotName := strings.TrimSpace(request.SnapshotName)
		if !validIdentifier(snapshotName) {
			return "", newCertainError(ErrorInvalid, false, "快照回滚需要有效的 SnapshotName", nil, 0)
		}
	}

	requestedNode := strings.TrimSpace(request.PVENode)
	if requestedNode != "" && !validIdentifier(requestedNode) {
		return "", newCertainError(ErrorInvalid, false, "PVE 节点名称无效", nil, 0)
	}
	node, err := adapter.resolveManagedVMNode(ctx, request.PVEVMID)
	if err != nil {
		return "", err
	}
	if requestedNode != "" && requestedNode != node {
		return "", newCertainError(ErrorOutOfScope, false, "目标节点与受管 PVE Pool 记录不一致", nil, 0)
	}

	switch request.Type {
	case domain.OperationPrecheck:
		return encodePrecheckTask(node, request.PVEVMID), nil
	case domain.OperationStart:
		return adapter.submitTask(ctx, []string{"nodes", node, "qemu", strconv.Itoa(request.PVEVMID), "status", "start"})
	case domain.OperationShutdown:
		return adapter.submitTask(ctx, []string{"nodes", node, "qemu", strconv.Itoa(request.PVEVMID), "status", "shutdown"})
	case domain.OperationRestore:
		return adapter.submitTask(ctx, []string{"nodes", node, "qemu", strconv.Itoa(request.PVEVMID), "snapshot", strings.TrimSpace(request.SnapshotName), "rollback"})
	default:
		return "", newCertainError(ErrorInvalid, false, "不支持的 PVE 操作类型", nil, 0)
	}
}

func (adapter *HTTPAdapter) Wait(ctx context.Context, upid string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, contextError(err, false, true)
	}
	if strings.HasPrefix(upid, precheckTaskPrefix) {
		return adapter.waitPrecheck(ctx, upid)
	}

	node, ok := nodeFromUPID(upid)
	if !ok {
		return Result{}, &Error{
			Code:      ErrorUnknownTask,
			Retryable: false,
			Message:   "PVE UPID 格式无效",
		}
	}

	waitContext, cancel := context.WithTimeout(ctx, adapter.taskTimeout)
	defer cancel()
	for {
		status, err := adapter.readTaskStatus(waitContext, node, upid)
		if err != nil {
			if waitContext.Err() != nil {
				return Result{}, contextError(waitContext.Err(), false, true)
			}
			var adapterError *Error
			if errors.As(err, &adapterError) && adapterError.Retryable {
				if sleepErr := sleepContext(waitContext, adapter.pollInterval); sleepErr != nil {
					return Result{}, contextError(sleepErr, false, true)
				}
				continue
			}
			return Result{}, err
		}

		switch strings.ToLower(strings.TrimSpace(status.Status)) {
		case "running":
			if err := sleepContext(waitContext, adapter.pollInterval); err != nil {
				return Result{}, contextError(err, false, true)
			}
		case "stopped":
			exitStatus := strings.TrimSpace(status.ExitStatus)
			if exitStatus == "" {
				return Result{}, uncertainError("PVE 任务已停止，但未返回最终状态", nil)
			}
			if strings.EqualFold(exitStatus, "OK") {
				return Result{Succeeded: true, Code: "OK", Message: "PVE 任务已完成"}, nil
			}
			return Result{Succeeded: false, Code: string(ErrorTaskFailed), Message: "PVE 任务执行失败"}, nil
		default:
			return Result{}, uncertainError("PVE 返回了未知的任务状态", nil)
		}
	}
}

func (adapter *HTTPAdapter) resolveManagedVMNode(ctx context.Context, vmid int) (string, error) {
	data, err := adapter.requestData(ctx, http.MethodGet, []string{"pools", adapter.managedPool}, nil, nil, purposePreflight)
	if err != nil {
		return "", err
	}
	var pool managedPool
	if err := json.Unmarshal(data, &pool); err != nil {
		return "", newCertainError(ErrorUnavailable, true, "无法解析受管 PVE Pool", err, 0)
	}
	for _, resource := range pool.Members {
		if resource.VMID != vmid {
			continue
		}
		if resource.Type != "qemu" {
			return "", newCertainError(ErrorOutOfScope, false, "目标 VMID 不是受管 QEMU 虚拟机", nil, 0)
		}
		if !validIdentifier(resource.Node) {
			return "", newCertainError(ErrorUnavailable, true, "PVE 返回了无效的节点名称", nil, 0)
		}
		return resource.Node, nil
	}
	return "", newCertainError(ErrorOutOfScope, false, "目标虚拟机不在配置的 PVE Pool 中", nil, 0)
}

func (adapter *HTTPAdapter) submitTask(ctx context.Context, path []string) (string, error) {
	data, err := adapter.requestData(ctx, http.MethodPost, path, nil, url.Values{}, purposeMutation)
	if err != nil {
		return "", err
	}
	var upid string
	if err := json.Unmarshal(data, &upid); err != nil || !validUPID(upid) {
		if err == nil {
			err = errors.New("PVE response did not contain a valid UPID")
		}
		return "", uncertainError("PVE 已响应，但未返回有效 UPID", err)
	}
	return upid, nil
}

func (adapter *HTTPAdapter) readTaskStatus(ctx context.Context, node, upid string) (taskStatus, error) {
	data, err := adapter.requestData(ctx, http.MethodGet, []string{"nodes", node, "tasks", upid, "status"}, nil, nil, purposeTaskPoll)
	if err != nil {
		return taskStatus{}, err
	}
	var status taskStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return taskStatus{}, &Error{
			Code:      ErrorUnavailable,
			Retryable: true,
			Message:   "无法解析 PVE 任务状态",
			Cause:     err,
		}
	}
	return status, nil
}

func (adapter *HTTPAdapter) waitPrecheck(ctx context.Context, taskID string) (Result, error) {
	node, vmid, ok := decodePrecheckTask(taskID)
	if !ok {
		return Result{}, &Error{Code: ErrorUnknownTask, Message: "PVE 预检任务标识无效"}
	}
	waitContext, cancel := context.WithTimeout(ctx, adapter.taskTimeout)
	defer cancel()
	data, err := adapter.requestData(waitContext, http.MethodGet, []string{"nodes", node, "qemu", strconv.Itoa(vmid), "status", "current"}, nil, nil, purposeTaskPoll)
	if err != nil {
		if waitContext.Err() != nil {
			return Result{}, contextError(waitContext.Err(), false, true)
		}
		return Result{}, err
	}
	var status vmStatus
	if err := json.Unmarshal(data, &status); err != nil || strings.TrimSpace(status.Status) == "" {
		return Result{}, uncertainError("PVE 未返回有效的虚拟机状态", err)
	}
	return Result{Succeeded: true, Code: "OK", Message: "PVE 虚拟机状态检查通过"}, nil
}

func (adapter *HTTPAdapter) requestData(
	ctx context.Context,
	method string,
	path []string,
	query url.Values,
	form url.Values,
	purpose requestPurpose,
) (json.RawMessage, error) {
	requestContext, cancel := context.WithTimeout(ctx, adapter.requestTimeout)
	defer cancel()
	if err := requestContext.Err(); err != nil {
		return nil, contextError(err, purpose == purposePreflight, purpose != purposeMutation)
	}

	requestURL := *adapter.baseURL
	requestURL.Path = joinURLPath(requestURL.Path, path)
	requestURL.RawPath = ""
	if len(query) > 0 {
		requestURL.RawQuery = query.Encode()
	}

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(requestContext, method, requestURL.String(), body)
	if err != nil {
		return nil, newCertainError(ErrorInvalid, false, "无法构造 PVE 请求", err, 0)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "PVEAPIToken="+adapter.tokenID+"="+adapter.tokenSecret)
	request.Header.Set("User-Agent", "PVE-Longxin64/1")
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	response, err := adapter.client.Do(request)
	if err != nil {
		return nil, adapter.transportError(err, purpose)
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, maxResponseBody+1)
	responseBody, readErr := io.ReadAll(limited)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, httpStatusError(response.StatusCode, purpose)
	}
	if readErr != nil {
		if purpose == purposeMutation {
			return nil, uncertainError("PVE 响应在返回 UPID 前中断", readErr)
		}
		return nil, &Error{
			Code:           ErrorUnavailable,
			Retryable:      true,
			Message:        "读取 PVE 响应失败",
			Cause:          readErr,
			OutcomeCertain: purpose == purposePreflight,
		}
	}
	if len(responseBody) > maxResponseBody {
		if purpose == purposeMutation {
			return nil, uncertainError("PVE 响应过大，无法确认任务结果", nil)
		}
		return nil, &Error{
			Code:           ErrorUnavailable,
			Retryable:      true,
			Message:        "PVE 响应超过大小限制",
			OutcomeCertain: purpose == purposePreflight,
		}
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		if purpose == purposeMutation {
			return nil, uncertainError("PVE 返回了无效响应，无法确认任务结果", err)
		}
		return nil, &Error{
			Code:           ErrorUnavailable,
			Retryable:      true,
			Message:        "PVE 返回了无效 JSON",
			Cause:          err,
			OutcomeCertain: purpose == purposePreflight,
		}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		if purpose == purposeMutation {
			return nil, uncertainError("PVE 已响应，但未返回任务标识", nil)
		}
		return nil, &Error{
			Code:           ErrorUnavailable,
			Retryable:      true,
			Message:        "PVE 响应缺少 data 字段",
			OutcomeCertain: purpose == purposePreflight,
		}
	}
	return envelope.Data, nil
}

func (adapter *HTTPAdapter) transportError(err error, purpose requestPurpose) error {
	var certificateVerificationError *tls.CertificateVerificationError
	var unknownAuthorityError x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	if errors.As(err, &certificateVerificationError) ||
		errors.As(err, &unknownAuthorityError) ||
		errors.As(err, &hostnameError) {
		return &Error{
			Code:           ErrorTLS,
			Retryable:      false,
			Message:        "PVE TLS 证书校验失败",
			Cause:          err,
			OutcomeCertain: true,
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{
			Code:           ErrorTimeout,
			Retryable:      purpose != purposeMutation,
			Message:        "PVE 请求超时",
			Cause:          context.DeadlineExceeded,
			OutcomeCertain: purpose == purposePreflight,
		}
	}
	if errors.Is(err, context.Canceled) {
		return &Error{
			Code:           ErrorCancelled,
			Retryable:      false,
			Message:        "PVE 请求已取消",
			Cause:          context.Canceled,
			OutcomeCertain: purpose == purposePreflight,
		}
	}
	return &Error{
		Code:           ErrorUnavailable,
		Retryable:      purpose != purposeMutation,
		Message:        "无法连接 PVE API",
		Cause:          err,
		OutcomeCertain: purpose == purposePreflight,
	}
}

func httpStatusError(status int, purpose requestPurpose) error {
	base := Error{HTTPStatus: status, OutcomeCertain: purpose != purposeTaskPoll}
	switch status {
	case http.StatusUnauthorized:
		base.Code = ErrorAuthentication
		base.Message = "PVE API Token 认证失败"
	case http.StatusForbidden:
		base.Code = ErrorPermission
		base.Message = "PVE API Token 权限不足"
	case http.StatusNotFound:
		if purpose == purposeTaskPoll {
			base.Code = ErrorUnknownTask
			base.Message = "PVE 任务不存在或已过期"
		} else {
			base.Code = ErrorNotFound
			base.Message = "PVE 资源或 API 路径不存在"
		}
	case http.StatusBadRequest, http.StatusMethodNotAllowed, http.StatusConflict, http.StatusUnprocessableEntity:
		base.Code = ErrorInvalid
		base.Message = "PVE 拒绝了无效请求"
	default:
		base.OutcomeCertain = purpose == purposePreflight
		if status == http.StatusRequestTimeout {
			base.Code = ErrorTimeout
			base.Message = "PVE 请求超时"
		} else {
			base.Code = ErrorUnavailable
			base.Message = "PVE API 暂时不可用"
		}
		base.Retryable = purpose != purposeMutation && (status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= http.StatusInternalServerError)
	}
	return &base
}

func contextError(err error, outcomeCertain, retryable bool) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{
			Code:           ErrorTimeout,
			Retryable:      retryable,
			Message:        "PVE 请求超时",
			Cause:          context.DeadlineExceeded,
			OutcomeCertain: outcomeCertain,
		}
	}
	return &Error{
		Code:           ErrorCancelled,
		Retryable:      false,
		Message:        "PVE 请求已取消",
		Cause:          context.Canceled,
		OutcomeCertain: outcomeCertain,
	}
}

func newCertainError(code ErrorCode, retryable bool, message string, cause error, status int) *Error {
	return &Error{
		Code:           code,
		Retryable:      retryable,
		Message:        message,
		Cause:          cause,
		HTTPStatus:     status,
		OutcomeCertain: true,
	}
}

func uncertainError(message string, cause error) *Error {
	return &Error{
		Code:      ErrorOutcomeUncertain,
		Retryable: false,
		Message:   message,
		Cause:     cause,
	}
}

func normalizeBaseURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, errors.New("PVE base URL is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse PVE base URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("PVE base URL must use https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("PVE base URL must contain only an HTTPS origin and optional path")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/api2/json"
	} else if !strings.HasSuffix(path, "/api2/json") {
		path += "/api2/json"
	}
	parsed.Path = path
	parsed.RawPath = ""
	return parsed, nil
}

func validateTokenPart(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("PVE %s is required", name)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("PVE %s contains invalid control characters", name)
	}
	return nil
}

func joinURLPath(base string, segments []string) string {
	path := strings.TrimRight(base, "/")
	for _, segment := range segments {
		path += "/" + url.PathEscape(segment)
	}
	return path
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			index > 0 && (character == '-' || character == '_' || character == '.') {
			continue
		}
		return false
	}
	return true
}

func validUPID(upid string) bool {
	_, ok := nodeFromUPID(upid)
	return ok
}

func nodeFromUPID(upid string) (string, bool) {
	if strings.ContainsAny(upid, "/?#\r\n") {
		return "", false
	}
	parts := strings.Split(upid, ":")
	if len(parts) < 3 || parts[0] != "UPID" || !validIdentifier(parts[1]) {
		return "", false
	}
	return parts[1], true
}

func encodePrecheckTask(node string, vmid int) string {
	encodedNode := base64.RawURLEncoding.EncodeToString([]byte(node))
	return precheckTaskPrefix + encodedNode + ":" + strconv.Itoa(vmid)
}

func decodePrecheckTask(taskID string) (string, int, bool) {
	payload := strings.TrimPrefix(taskID, precheckTaskPrefix)
	if payload == taskID {
		return "", 0, false
	}
	separator := strings.LastIndexByte(payload, ':')
	if separator <= 0 || separator == len(payload)-1 {
		return "", 0, false
	}
	nodeBytes, err := base64.RawURLEncoding.DecodeString(payload[:separator])
	if err != nil || !validIdentifier(string(nodeBytes)) {
		return "", 0, false
	}
	vmid, err := strconv.Atoi(payload[separator+1:])
	if err != nil || vmid <= 0 {
		return "", 0, false
	}
	return string(nodeBytes), vmid, true
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
