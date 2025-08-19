package types

import (
	"errors"
	"fmt"
)

// Common error variables
var (
	// Task errors
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskInvalidStatus = errors.New("invalid task status")
	ErrTaskTimeout       = errors.New("task execution timeout")
	ErrTaskCancelled     = errors.New("task was cancelled")
	ErrTaskDeadline      = errors.New("task deadline exceeded")
	ErrTaskRetryExceeded = errors.New("maximum retries exceeded")
	ErrTaskDuplicate     = errors.New("duplicate task detected")
	
	// Queue errors
	ErrQueueNotFound     = errors.New("queue not found")
	ErrQueueFull         = errors.New("queue is full")
	ErrQueueEmpty        = errors.New("queue is empty")
	ErrQueueClosed       = errors.New("queue is closed")
	
	// Worker errors
	ErrWorkerNotFound    = errors.New("worker not found")
	ErrWorkerOffline     = errors.New("worker is offline")
	ErrWorkerOverloaded  = errors.New("worker is overloaded")
	ErrWorkerShutdown    = errors.New("worker is shutting down")
	
	// Processor errors
	ErrProcessorNotFound = errors.New("no processor found for task type")
	ErrProcessorFailed   = errors.New("task processor failed")
	
	// Validation errors
	ErrInvalidPayload    = errors.New("invalid task payload")
	ErrInvalidPriority   = errors.New("invalid task priority")
	ErrInvalidTaskType   = errors.New("invalid task type")
	ErrMissingRequired   = errors.New("missing required field")
	
	// Backend errors
	ErrBackendUnavailable = errors.New("backend is unavailable")
	ErrBackendTimeout     = errors.New("backend operation timeout")
	ErrBackendConnection  = errors.New("backend connection error")
	
	// Rate limiting errors
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
	
	// Circuit breaker errors
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
)

// TaskError represents a task-specific error with additional context
type TaskError struct {
	TaskID    string    `json:"task_id"`
	TaskType  TaskType  `json:"task_type"`
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
	Retryable bool      `json:"retryable"`
	Cause     error     `json:"-"` // Original error (not serialized)
}

func (e *TaskError) Error() string {
	if e.TaskID != "" {
		return fmt.Sprintf("task %s (%s): %s", e.TaskID, e.TaskType, e.Message)
	}
	return fmt.Sprintf("task (%s): %s", e.TaskType, e.Message)
}

func (e *TaskError) Unwrap() error {
	return e.Cause
}

// NewTaskError creates a new TaskError
func NewTaskError(taskID string, taskType TaskType, code ErrorCode, message string) *TaskError {
	return &TaskError{
		TaskID:   taskID,
		TaskType: taskType,
		Code:     code,
		Message:  message,
		Retryable: code.IsRetryable(),
	}
}

// NewTaskErrorWithCause creates a new TaskError with an underlying cause
func NewTaskErrorWithCause(taskID string, taskType TaskType, code ErrorCode, message string, cause error) *TaskError {
	return &TaskError{
		TaskID:   taskID,
		TaskType: taskType,
		Code:     code,
		Message:  message,
		Cause:    cause,
		Retryable: code.IsRetryable(),
	}
}

// ErrorCode represents specific error types with retry behavior
type ErrorCode string

const (
	// Retryable errors (temporary failures)
	ErrorCodeTimeout           ErrorCode = "timeout"
	ErrorCodeNetworkError      ErrorCode = "network_error"
	ErrorCodeBackendUnavailable ErrorCode = "backend_unavailable"
	ErrorCodeRateLimited       ErrorCode = "rate_limited"
	ErrorCodeCircuitOpen       ErrorCode = "circuit_open"
	ErrorCodeWorkerOverloaded  ErrorCode = "worker_overloaded"
	ErrorCodeTemporaryFailure  ErrorCode = "temporary_failure"
	
	// Non-retryable errors (permanent failures)
	ErrorCodeInvalidPayload    ErrorCode = "invalid_payload"
	ErrorCodeValidationFailed  ErrorCode = "validation_failed"
	ErrorCodeUnauthorized      ErrorCode = "unauthorized"
	ErrorCodeForbidden         ErrorCode = "forbidden"
	ErrorCodeNotFound          ErrorCode = "not_found"
	ErrorCodeDuplicateTask     ErrorCode = "duplicate_task"
	ErrorCodeDeadlineExceeded  ErrorCode = "deadline_exceeded"
	ErrorCodeCancelled         ErrorCode = "cancelled"
	ErrorCodePermanentFailure  ErrorCode = "permanent_failure"
	
	// System errors
	ErrorCodeInternalError     ErrorCode = "internal_error"
	ErrorCodeConfigError       ErrorCode = "config_error"
	ErrorCodeUnknownError      ErrorCode = "unknown_error"
)

// IsRetryable returns true if the error code indicates a retryable failure
func (e ErrorCode) IsRetryable() bool {
	switch e {
	case ErrorCodeTimeout, ErrorCodeNetworkError, ErrorCodeBackendUnavailable,
		 ErrorCodeRateLimited, ErrorCodeCircuitOpen, ErrorCodeWorkerOverloaded,
		 ErrorCodeTemporaryFailure:
		return true
	default:
		return false
	}
}

// ValidationError represents field validation errors
type ValidationError struct {
	Field   string `json:"field"`
	Value   interface{} `json:"value"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []*ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "validation failed"
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	return fmt.Sprintf("validation failed: %d errors", len(e))
}

// Constants for default values
const (
	// Default retry settings
	DefaultMaxRetries     = 3
	DefaultRetryBackoff   = "exponential" // exponential, linear, fixed
	DefaultInitialDelay   = "1s"
	DefaultMaxDelay       = "5m"
	DefaultBackoffFactor  = 2.0
	
	// Default timeouts
	DefaultTaskTimeout    = "5m"
	DefaultDequeueTimeout = "30s"
	DefaultShutdownTimeout = "30s"
	
	// Default queue settings
	DefaultQueueName      = "default"
	DefaultPriority       = PriorityNormal
	DefaultBatchSize      = 10
	DefaultMaxQueueSize   = 10000
	
	// Default worker settings
	DefaultWorkerConcurrency = 5
	DefaultHeartbeatInterval = "30s"
	DefaultWorkerTimeout     = "5m"
	
	// Circuit breaker defaults
	DefaultCircuitBreakerThreshold    = 5     // failures before opening
	DefaultCircuitBreakerTimeout      = "60s" // how long to stay open
	DefaultCircuitBreakerMaxRequests  = 3     // max requests in half-open state
	
	// Rate limiting defaults
	DefaultRateLimit      = 100  // requests per second
	DefaultRateBurst      = 10   // burst size
	
	// Monitoring defaults
	DefaultMetricsPort    = 9090
	DefaultHealthPath     = "/health"
	DefaultMetricsPath    = "/metrics"
	DefaultReadyPath      = "/ready"
)

// QueuePriorities defines the processing order for different priority levels
var QueuePriorities = []Priority{
	PriorityCritical,
	PriorityHigh,
	PriorityNormal,
	PriorityLow,
}

// SupportedTaskTypes lists all supported task types
var SupportedTaskTypes = []TaskType{
	TaskTypeWebhook,
	TaskTypeEmail,
	TaskTypeImageProcess,
	TaskTypeDataProcess,
	TaskTypeScheduled,
	TaskTypeBatch,
}

// RetryableErrorCodes lists all error codes that should trigger retries
var RetryableErrorCodes = []ErrorCode{
	ErrorCodeTimeout,
	ErrorCodeNetworkError,
	ErrorCodeBackendUnavailable,
	ErrorCodeRateLimited,
	ErrorCodeCircuitOpen,
	ErrorCodeWorkerOverloaded,
	ErrorCodeTemporaryFailure,
}