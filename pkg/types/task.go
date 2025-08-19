package types

import (
	"encoding/json"
	"time"
)

// Priority defines task execution priority levels
type Priority string

const (
	PriorityCritical Priority = "critical" // Financial calculations, payment processing
	PriorityHigh     Priority = "high"     // User-facing operations, webhooks
	PriorityNormal   Priority = "normal"   // Background processing, reports
	PriorityLow      Priority = "low"      // Analytics, cleanup tasks
)

// TaskType defines the type of task to be executed
type TaskType string

const (
	TaskTypeWebhook       TaskType = "webhook"         // HTTP webhook delivery
	TaskTypeEmail         TaskType = "email"           // Email processing
	TaskTypeImageProcess  TaskType = "image_process"   // Image operations
	TaskTypeDataProcess   TaskType = "data_process"    // Data transformations
	TaskTypeScheduled     TaskType = "scheduled"       // Cron/scheduled tasks
	TaskTypeBatch         TaskType = "batch"           // Bulk operations
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"     // Queued, awaiting processing
	TaskStatusRunning    TaskStatus = "running"     // Currently being processed
	TaskStatusCompleted  TaskStatus = "completed"   // Successfully completed
	TaskStatusFailed     TaskStatus = "failed"      // Failed (will retry if attempts remain)
	TaskStatusDeadLetter TaskStatus = "dead_letter" // Failed permanently, moved to DLQ
	TaskStatusCancelled  TaskStatus = "cancelled"   // Cancelled by user
)

// Task represents a unit of work in the TaskForge system
type Task struct {
	// Core identifiers
	ID          string    `json:"id" bson:"_id"`                    // Unique task identifier
	Type        TaskType  `json:"type" bson:"type"`                 // Type of task
	Priority    Priority  `json:"priority" bson:"priority"`         // Execution priority
	Status      TaskStatus `json:"status" bson:"status"`            // Current status
	Queue       string    `json:"queue" bson:"queue"`               // Target queue name
	
	// Payload and metadata
	Payload     json.RawMessage `json:"payload" bson:"payload"`       // Task-specific data
	Metadata    map[string]interface{} `json:"metadata" bson:"metadata"` // Additional metadata
	
	// Scheduling and timing
	CreatedAt   time.Time  `json:"created_at" bson:"created_at"`     // When task was created
	ScheduledAt *time.Time `json:"scheduled_at" bson:"scheduled_at"` // When to execute (nil = immediate)
	StartedAt   *time.Time `json:"started_at" bson:"started_at"`     // When processing began
	CompletedAt *time.Time `json:"completed_at" bson:"completed_at"` // When processing finished
	
	// Retry and error handling
	MaxRetries     int       `json:"max_retries" bson:"max_retries"`         // Maximum retry attempts
	CurrentRetries int       `json:"current_retries" bson:"current_retries"` // Current retry count
	LastError      string    `json:"last_error,omitempty" bson:"last_error"` // Last error message
	NextRetryAt    *time.Time `json:"next_retry_at" bson:"next_retry_at"`     // When to retry next
	
	// Processing context
	WorkerID     string `json:"worker_id,omitempty" bson:"worker_id"`       // ID of processing worker
	CorrelationID string `json:"correlation_id,omitempty" bson:"correlation_id"` // For request tracing
	
	// Multi-tenancy and isolation
	TenantID     string `json:"tenant_id,omitempty" bson:"tenant_id"`       // Tenant identifier
	
	// Timeout and deadlines
	Timeout      *time.Duration `json:"timeout,omitempty" bson:"timeout"`   // Max execution time
	DeadlineAt   *time.Time     `json:"deadline_at" bson:"deadline_at"`     // Hard deadline
	
	// Deduplication
	DedupeKey    string `json:"dedupe_key,omitempty" bson:"dedupe_key"`     // For preventing duplicates
}

// TaskResult represents the outcome of task execution
type TaskResult struct {
	TaskID      string                 `json:"task_id"`
	Status      TaskStatus             `json:"status"`
	Result      json.RawMessage        `json:"result,omitempty"`      // Success result data
	Error       string                 `json:"error,omitempty"`       // Error message if failed
	Duration    time.Duration          `json:"duration"`              // Execution time
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // Additional result metadata
	CompletedAt time.Time              `json:"completed_at"`
	WorkerID    string                 `json:"worker_id"`
}

// TaskOptions provides configuration for task creation
type TaskOptions struct {
	Priority      Priority               `json:"priority,omitempty"`
	Queue         string                 `json:"queue,omitempty"`
	MaxRetries    *int                   `json:"max_retries,omitempty"`
	Timeout       *time.Duration         `json:"timeout,omitempty"`
	ScheduledAt   *time.Time             `json:"scheduled_at,omitempty"`
	DeadlineAt    *time.Time             `json:"deadline_at,omitempty"`
	DedupeKey     string                 `json:"dedupe_key,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	TenantID      string                 `json:"tenant_id,omitempty"`
}

// QueueStats provides statistics about a queue
type QueueStats struct {
	QueueName       string            `json:"queue_name"`
	PendingTasks    int64             `json:"pending_tasks"`
	RunningTasks    int64             `json:"running_tasks"`
	CompletedTasks  int64             `json:"completed_tasks"`
	FailedTasks     int64             `json:"failed_tasks"`
	DeadLetterTasks int64             `json:"dead_letter_tasks"`
	TasksByPriority map[Priority]int64 `json:"tasks_by_priority"`
	TasksByType     map[TaskType]int64 `json:"tasks_by_type"`
	LastUpdated     time.Time         `json:"last_updated"`
}

// WorkerInfo represents information about a worker
type WorkerInfo struct {
	ID           string            `json:"id"`
	Hostname     string            `json:"hostname"`
	Version      string            `json:"version"`
	Queues       []string          `json:"queues"`           // Queues this worker processes
	Status       WorkerStatus      `json:"status"`
	RegisteredAt time.Time         `json:"registered_at"`
	LastHeartbeat time.Time        `json:"last_heartbeat"`
	CurrentTasks []string          `json:"current_tasks"`    // Currently processing task IDs
	Capabilities []string          `json:"capabilities"`     // Task types this worker can handle
	Metadata     map[string]string `json:"metadata"`
}

// WorkerStatus represents the current state of a worker
type WorkerStatus string

const (
	WorkerStatusIdle       WorkerStatus = "idle"        // Available for work
	WorkerStatusBusy       WorkerStatus = "busy"        // Processing tasks
	WorkerStatusDraining   WorkerStatus = "draining"    // Finishing current tasks, no new ones
	WorkerStatusOffline    WorkerStatus = "offline"     // Disconnected
	WorkerStatusMaintenance WorkerStatus = "maintenance" // Under maintenance
)