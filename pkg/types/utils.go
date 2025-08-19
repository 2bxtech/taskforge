package types

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"
	
	"github.com/google/uuid"
)

// TaskIDGenerator generates unique task IDs
type TaskIDGenerator interface {
	Generate() string
}

// UUIDGenerator generates UUID-based task IDs
type UUIDGenerator struct{}

func (g *UUIDGenerator) Generate() string {
	return uuid.New().String()
}

// PrefixedIDGenerator generates IDs with a prefix
type PrefixedIDGenerator struct {
	Prefix string
}

func (g *PrefixedIDGenerator) Generate() string {
	id := uuid.New().String()
	if g.Prefix != "" {
		return fmt.Sprintf("%s_%s", g.Prefix, id)
	}
	return id
}

// GenerateCorrelationID generates a correlation ID for request tracing
func GenerateCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return fmt.Sprintf("corr_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// GenerateDedupeKey generates a deduplication key based on task content
func GenerateDedupeKey(taskType TaskType, payload []byte) string {
	// Simple implementation - in production, you might want to use a hash
	return fmt.Sprintf("%s_%x", taskType, payload)
}

// CalculateNextRetry calculates the next retry time based on backoff strategy
func CalculateNextRetry(currentRetry int, strategy string, initialDelay, maxDelay time.Duration, factor float64) time.Time {
	var delay time.Duration
	
	switch strategy {
	case "exponential":
		// Proper exponential backoff: initialDelay * factor^currentRetry
		delay = time.Duration(float64(initialDelay) * math.Pow(factor, float64(currentRetry)))
	case "linear":
		delay = time.Duration(int64(initialDelay) * int64(currentRetry+1))
	case "fixed":
		delay = initialDelay
	default:
		delay = initialDelay
	}
	
	if delay > maxDelay {
		delay = maxDelay
	}
	
	return time.Now().Add(delay)
}

// IsValidPriority checks if a priority is valid
func IsValidPriority(priority Priority) bool {
	switch priority {
	case PriorityCritical, PriorityHigh, PriorityNormal, PriorityLow:
		return true
	default:
		return false
	}
}

// IsValidTaskType checks if a task type is valid
func IsValidTaskType(taskType TaskType) bool {
	for _, supportedType := range SupportedTaskTypes {
		if taskType == supportedType {
			return true
		}
	}
	return false
}

// IsValidTaskStatus checks if a task status is valid
func IsValidTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusPending, TaskStatusRunning, TaskStatusCompleted, 
		 TaskStatusFailed, TaskStatusDeadLetter, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// IsTerminalStatus checks if a task status is terminal (won't change)
func IsTerminalStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusCompleted, TaskStatusDeadLetter, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// GetPriorityWeight returns a numeric weight for priority-based sorting
func GetPriorityWeight(priority Priority) int {
	switch priority {
	case PriorityCritical:
		return 4
	case PriorityHigh:
		return 3
	case PriorityNormal:
		return 2
	case PriorityLow:
		return 1
	default:
		return 0
	}
}

// TaskBuilder provides a fluent interface for building tasks
type TaskBuilder struct {
	task *Task
	err  error // Store any errors during building
}

// NewTaskBuilder creates a new task builder
func NewTaskBuilder(taskType TaskType) *TaskBuilder {
	return &TaskBuilder{
		task: &Task{
			ID:        uuid.New().String(),
			Type:      taskType,
			Priority:  PriorityNormal,
			Status:    TaskStatusPending,
			Queue:     DefaultQueueName,
			CreatedAt: time.Now(),
			Metadata:  make(map[string]interface{}),
		},
	}
}

// WithID sets the task ID
func (b *TaskBuilder) WithID(id string) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.ID = id
	return b
}

// WithPriority sets the task priority
func (b *TaskBuilder) WithPriority(priority Priority) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.Priority = priority
	return b
}

// WithQueue sets the target queue
func (b *TaskBuilder) WithQueue(queue string) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.Queue = queue
	return b
}

// WithPayload sets the task payload
// The payload must be a valid JSON-serializable value
func (b *TaskBuilder) WithPayload(payload interface{}) *TaskBuilder {
	if b.err != nil {
		return b
	}
	
	// Serialize the payload to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		b.err = fmt.Errorf("failed to marshal payload to JSON: %w", err)
		return b
	}
	b.task.Payload = data
	return b
}

// WithRawPayload sets the task payload from already-serialized JSON
func (b *TaskBuilder) WithRawPayload(payload json.RawMessage) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.Payload = payload
	return b
}

// WithScheduledAt sets when the task should be executed
func (b *TaskBuilder) WithScheduledAt(scheduledAt time.Time) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.ScheduledAt = &scheduledAt
	return b
}

// WithMaxRetries sets the maximum retry attempts
func (b *TaskBuilder) WithMaxRetries(maxRetries int) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.MaxRetries = maxRetries
	return b
}

// WithTimeout sets the task timeout
func (b *TaskBuilder) WithTimeout(timeout time.Duration) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.Timeout = &timeout
	return b
}

// WithDeadline sets the task deadline
func (b *TaskBuilder) WithDeadline(deadline time.Time) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.DeadlineAt = &deadline
	return b
}

// WithDedupeKey sets the deduplication key
func (b *TaskBuilder) WithDedupeKey(key string) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.DedupeKey = key
	return b
}

// WithCorrelationID sets the correlation ID
func (b *TaskBuilder) WithCorrelationID(correlationID string) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.CorrelationID = correlationID
	return b
}

// WithTenantID sets the tenant ID
func (b *TaskBuilder) WithTenantID(tenantID string) *TaskBuilder {
	if b.err != nil {
		return b
	}
	b.task.TenantID = tenantID
	return b
}

// WithMetadata adds metadata to the task
func (b *TaskBuilder) WithMetadata(key string, value interface{}) *TaskBuilder {
	if b.err != nil {
		return b
	}
	if b.task.Metadata == nil {
		b.task.Metadata = make(map[string]interface{})
	}
	b.task.Metadata[key] = value
	return b
}

// Build returns the constructed task or an error if building failed
func (b *TaskBuilder) Build() (*Task, error) {
	if b.err != nil {
		return nil, b.err
	}
	
	// Set defaults if not provided
	if b.task.MaxRetries == 0 {
		b.task.MaxRetries = DefaultMaxRetries
	}
	
	// Generate correlation ID if not provided
	if b.task.CorrelationID == "" {
		b.task.CorrelationID = GenerateCorrelationID()
	}
	
	return b.task, nil
}

// MustBuild returns the constructed task or panics if there's an error
// Use this only in tests or when you're certain the build will succeed
func (b *TaskBuilder) MustBuild() *Task {
	task, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build task: %v", err))
	}
	return task
}

// Clone creates a deep copy of a task
func (t *Task) Clone() *Task {
	clone := *t
	
	// Deep copy slices and maps
	if t.Metadata != nil {
		clone.Metadata = make(map[string]interface{})
		for k, v := range t.Metadata {
			clone.Metadata[k] = v
		}
	}
	
	// Copy payload
	if t.Payload != nil {
		clone.Payload = make([]byte, len(t.Payload))
		copy(clone.Payload, t.Payload)
	}
	
	// Copy time pointers
	if t.ScheduledAt != nil {
		scheduledAt := *t.ScheduledAt
		clone.ScheduledAt = &scheduledAt
	}
	
	if t.StartedAt != nil {
		startedAt := *t.StartedAt
		clone.StartedAt = &startedAt
	}
	
	if t.CompletedAt != nil {
		completedAt := *t.CompletedAt
		clone.CompletedAt = &completedAt
	}
	
	if t.NextRetryAt != nil {
		nextRetryAt := *t.NextRetryAt
		clone.NextRetryAt = &nextRetryAt
	}
	
	if t.DeadlineAt != nil {
		deadlineAt := *t.DeadlineAt
		clone.DeadlineAt = &deadlineAt
	}
	
	if t.Timeout != nil {
		timeout := *t.Timeout
		clone.Timeout = &timeout
	}
	
	return &clone
}

// IsExpired checks if a task has exceeded its deadline
func (t *Task) IsExpired() bool {
	return t.DeadlineAt != nil && time.Now().After(*t.DeadlineAt)
}

// CanRetry checks if a task can be retried
func (t *Task) CanRetry() bool {
	return t.CurrentRetries < t.MaxRetries && t.Status == TaskStatusFailed && !t.IsExpired()
}

// Duration returns how long the task took to complete
func (t *Task) Duration() time.Duration {
	if t.StartedAt == nil || t.CompletedAt == nil {
		return 0
	}
	return t.CompletedAt.Sub(*t.StartedAt)
}

// Age returns how long ago the task was created
func (t *Task) Age() time.Duration {
	return time.Since(t.CreatedAt)
}

// QueueTime returns how long the task waited in the queue before processing
func (t *Task) QueueTime() time.Duration {
	if t.StartedAt == nil {
		return time.Since(t.CreatedAt)
	}
	return t.StartedAt.Sub(t.CreatedAt)
}