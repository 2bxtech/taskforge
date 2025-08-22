package types

import (
	"context"
	"time"
)

// QueueBackend defines the interface for different queue implementations
// This allows us to swap between Redis, PostgreSQL, NATS, etc.
type QueueBackend interface {
	// Task operations
	Enqueue(ctx context.Context, task *Task) error
	Dequeue(ctx context.Context, queue string, timeout time.Duration) (*Task, error)
	Ack(ctx context.Context, taskID string) error
	Nack(ctx context.Context, taskID string, reason string) error

	// Batch operations for efficiency
	EnqueueBatch(ctx context.Context, tasks []*Task) error
	DequeueBatch(ctx context.Context, queue string, count int, timeout time.Duration) ([]*Task, error)

	// Task management
	GetTask(ctx context.Context, taskID string) (*Task, error)
	UpdateTask(ctx context.Context, task *Task) error
	DeleteTask(ctx context.Context, taskID string) error

	// Queue management
	GetQueueStats(ctx context.Context, queue string) (*QueueStats, error)
	ListQueues(ctx context.Context) ([]string, error)
	PurgeQueue(ctx context.Context, queue string) error

	// Dead letter queue operations
	MoveToDLQ(ctx context.Context, taskID string, reason string) error
	RequeueFromDLQ(ctx context.Context, taskID string) error

	// Retry and scheduling
	ScheduleRetry(ctx context.Context, taskID string, retryAt time.Time) error
	GetScheduledTasks(ctx context.Context, before time.Time, limit int) ([]*Task, error)

	// Health and monitoring
	HealthCheck(ctx context.Context) error
	Close() error
}

// TaskProcessor defines the interface for task execution
type TaskProcessor interface {
	// Process executes a task and returns the result
	Process(ctx context.Context, task *Task) (*TaskResult, error)

	// GetSupportedTypes returns the task types this processor can handle
	GetSupportedTypes() []TaskType

	// GetCapabilities returns additional capabilities (e.g., "image-resize", "webhook-v2")
	GetCapabilities() []string
}

// Worker defines the interface for task workers
type Worker interface {
	// Start begins processing tasks from specified queues
	Start(ctx context.Context, queues []string) error

	// Stop gracefully stops the worker, finishing current tasks
	Stop(ctx context.Context) error

	// RegisterProcessor adds a task processor for specific task types
	RegisterProcessor(taskType TaskType, processor TaskProcessor) error

	// GetInfo returns current worker information
	GetInfo() *WorkerInfo

	// Heartbeat updates the worker's status and metadata
	Heartbeat(ctx context.Context) error
}

// Scheduler defines the interface for task scheduling
type Scheduler interface {
	// Start begins the scheduling process
	Start(ctx context.Context) error

	// Stop gracefully stops the scheduler
	Stop(ctx context.Context) error

	// ScheduleTask adds a task to be executed at a specific time
	ScheduleTask(ctx context.Context, task *Task, executeAt time.Time) error

	// ScheduleCron adds a recurring task with cron expression
	ScheduleCron(ctx context.Context, cronExpr string, taskTemplate *Task) error

	// CancelScheduledTask removes a scheduled task
	CancelScheduledTask(ctx context.Context, taskID string) error

	// GetScheduledTasks returns upcoming scheduled tasks
	GetScheduledTasks(ctx context.Context, from, to time.Time) ([]*Task, error)
}

// MetricsCollector defines the interface for metrics collection
type MetricsCollector interface {
	// Task metrics
	RecordTaskEnqueued(taskType TaskType, priority Priority, queue string)
	RecordTaskStarted(taskType TaskType, priority Priority, queue string)
	RecordTaskCompleted(taskType TaskType, priority Priority, queue string, duration time.Duration)
	RecordTaskFailed(taskType TaskType, priority Priority, queue string, duration time.Duration, errorType string)

	// Queue metrics
	UpdateQueueDepth(queue string, depth int64)
	UpdateActiveWorkers(queue string, count int64)

	// Worker metrics
	RecordWorkerRegistered(workerID string, queues []string)
	RecordWorkerUnregistered(workerID string)
	UpdateWorkerStatus(workerID string, status WorkerStatus)

	// Circuit breaker metrics
	RecordCircuitBreakerOpen(service string)
	RecordCircuitBreakerClosed(service string)
	RecordCircuitBreakerHalfOpen(service string)
}

// Logger defines the interface for structured logging
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
}

// Field represents a structured log field
type Field struct {
	Key   string
	Value interface{}
}

// CircuitBreaker defines the interface for circuit breaker pattern
type CircuitBreaker interface {
	// Execute runs the function with circuit breaker protection
	Execute(fn func() error) error

	// State returns the current circuit breaker state
	State() CircuitBreakerState

	// Reset manually resets the circuit breaker
	Reset()
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState string

const (
	CircuitBreakerClosed   CircuitBreakerState = "closed"    // Normal operation
	CircuitBreakerOpen     CircuitBreakerState = "open"      // Failing fast
	CircuitBreakerHalfOpen CircuitBreakerState = "half_open" // Testing if service recovered
)

// RateLimiter defines the interface for rate limiting
type RateLimiter interface {
	// Allow checks if an operation is allowed under the rate limit
	Allow(ctx context.Context, key string) (bool, error)

	// AllowN checks if N operations are allowed
	AllowN(ctx context.Context, key string, n int) (bool, error)

	// Reset resets the rate limiter for a specific key
	Reset(ctx context.Context, key string) error
}

// TaskValidator defines the interface for task validation
type TaskValidator interface {
	// Validate checks if a task is valid and can be processed
	Validate(task *Task) error

	// ValidatePayload validates task-specific payload
	ValidatePayload(taskType TaskType, payload []byte) error
}

// TaskTransformer defines the interface for task transformation
type TaskTransformer interface {
	// Transform modifies a task before it's enqueued (e.g., add defaults, enrich data)
	Transform(task *Task) (*Task, error)

	// GetSupportedTypes returns the task types this transformer handles
	GetSupportedTypes() []TaskType
}
