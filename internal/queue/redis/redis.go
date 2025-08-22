package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
	"github.com/redis/go-redis/v9"
)

// Queue implements the QueueBackend interface using Redis Streams
type Queue struct {
	connMgr    *ConnectionManager
	config     *Config
	client     *redis.Client
	logger     types.Logger
	serializer *TaskSerializer
}

// NewRedisQueue creates a new Redis queue backend
func NewRedisQueue(config *Config, logger types.Logger) (*Queue, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create connection manager
	connMgr, err := NewConnectionManager(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection manager: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := connMgr.HealthCheck(ctx); err != nil {
		return nil, fmt.Errorf("redis health check failed: %w", err)
	}

	queue := &Queue{
		connMgr:    connMgr,
		config:     connMgr.GetConfig(),
		client:     connMgr.GetClient(),
		logger:     logger,
		serializer: NewTaskSerializer(),
	}

	// Initialize consumer groups for default queues
	if err := queue.initializeConsumerGroups(ctx); err != nil {
		logger.Warn("failed to initialize consumer groups", types.Field{Key: "error", Value: err})
	}

	return queue, nil
}

// Enqueue adds a task to the specified queue
func (r *Queue) Enqueue(ctx context.Context, task *types.Task) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}

	// Set default queue if not specified
	if task.Queue == "" {
		task.Queue = "default"
	}

	// Serialize task
	taskData, err := r.serializer.Serialize(task)
	if err != nil {
		return fmt.Errorf("failed to serialize task: %w", err)
	}

	return r.connMgr.WithRetry(ctx, func() error {
		// Use pipeline for atomic operations
		pipe := r.client.Pipeline()

		// Add to stream
		streamName := r.config.GetStreamName(task.Queue)
		streamFields := map[string]interface{}{
			"task_id":    task.ID,
			"task_type":  string(task.Type),
			"priority":   string(task.Priority),
			"tenant_id":  task.TenantID,
			"created_at": task.CreatedAt.Unix(),
			"payload":    taskData,
		}

		// Add scheduled time if present
		if task.ScheduledAt != nil {
			streamFields["scheduled_at"] = task.ScheduledAt.Unix()
		}

		// Add deadline if present
		if task.DeadlineAt != nil {
			streamFields["deadline_at"] = task.DeadlineAt.Unix()
		}

		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: streamName,
			MaxLen: r.config.MaxStreamLength,
			Approx: true,
			Values: streamFields,
		})

		// Add to priority queue
		priorityScore := r.calculatePriorityScore(task.Priority, task.CreatedAt)
		prioritySetName := r.config.GetPrioritySetName(task.Queue)
		pipe.ZAdd(ctx, prioritySetName, redis.Z{
			Score:  priorityScore,
			Member: task.ID,
		})

		// Store full task data
		taskHashKey := r.config.GetTaskHashKey(task.ID)
		pipe.HSet(ctx, taskHashKey, map[string]interface{}{
			"data":        taskData,
			"status":      string(task.Status),
			"enqueued_at": time.Now().Unix(),
		})

		// Set TTL for task data
		pipe.Expire(ctx, taskHashKey, r.config.TaskTTL)

		// Execute pipeline
		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to enqueue task: %w", err)
		}

		r.logger.Info("task enqueued successfully",
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "queue", Value: task.Queue},
			types.Field{Key: "priority", Value: task.Priority},
			types.Field{Key: "type", Value: task.Type},
		)

		return nil
	})
}

// Dequeue retrieves and claims a task from the specified queue
func (r *Queue) Dequeue(ctx context.Context, queue string, timeout time.Duration) (*types.Task, error) {
	if queue == "" {
		queue = "default"
	}

	// Ensure consumer group exists
	if err := r.ensureConsumerGroup(ctx, queue); err != nil {
		r.logger.Warn("failed to ensure consumer group",
			types.Field{Key: "queue", Value: queue},
			types.Field{Key: "error", Value: err},
		)
	}

	var task *types.Task
	err := r.connMgr.WithRetry(ctx, func() error {
		// First, try to claim pending messages
		claimedTask, err := r.claimPendingMessage(ctx, queue)
		if err != nil {
			return err
		}

		if claimedTask != nil {
			task = claimedTask
			return nil
		}

		// If no pending messages, read new messages
		newTask, err := r.readNewMessage(ctx, queue, timeout)
		if err != nil {
			return err
		}

		task = newTask
		return nil
	})

	if err != nil {
		return nil, err
	}

	if task != nil {
		r.logger.Info("task dequeued successfully",
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "queue", Value: queue},
			types.Field{Key: "type", Value: task.Type},
		)
	}

	return task, nil
}

// Ack acknowledges successful task completion
func (r *Queue) Ack(ctx context.Context, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task ID is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		// Get task to find queue and stream entry ID
		task, err := r.GetTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get task for ack: %w", err)
		}

		if task == nil {
			return fmt.Errorf("task not found: %s", taskID)
		}

		streamName := r.config.GetStreamName(task.Queue)

		// Find the stream entry ID for this task
		entryID, err := r.findStreamEntryID(ctx, streamName, taskID)
		if err != nil {
			return fmt.Errorf("failed to find stream entry: %w", err)
		}

		if entryID == "" {
			return fmt.Errorf("stream entry not found for task: %s", taskID)
		}

		// Use pipeline for atomic operations
		pipe := r.client.Pipeline()

		// Acknowledge the message in the stream
		pipe.XAck(ctx, streamName, r.config.ConsumerGroup, entryID)

		// Remove from priority queue
		prioritySetName := r.config.GetPrioritySetName(task.Queue)
		pipe.ZRem(ctx, prioritySetName, taskID)

		// Update task status
		taskHashKey := r.config.GetTaskHashKey(taskID)
		pipe.HSet(ctx, taskHashKey, "status", string(types.TaskStatusCompleted))
		pipe.HSet(ctx, taskHashKey, "completed_at", time.Now().Unix())

		// Execute pipeline
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to ack task: %w", err)
		}

		r.logger.Info("task acknowledged successfully",
			types.Field{Key: "task_id", Value: taskID},
			types.Field{Key: "queue", Value: task.Queue},
		)

		return nil
	})
}

// Nack negatively acknowledges a task (marks as failed)
func (r *Queue) Nack(ctx context.Context, taskID string, reason string) error {
	if taskID == "" {
		return fmt.Errorf("task ID is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		// Get task to determine retry logic
		task, err := r.GetTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get task for nack: %w", err)
		}

		if task == nil {
			return fmt.Errorf("task not found: %s", taskID)
		}

		// Determine if task should be retried or moved to DLQ
		shouldRetry := task.CurrentRetries < task.MaxRetries

		if shouldRetry {
			return r.scheduleRetry(ctx, task, reason)
		}
		return r.MoveToDLQ(ctx, taskID, reason)
	})
}

// EnqueueBatch enqueues multiple tasks atomically
func (r *Queue) EnqueueBatch(ctx context.Context, tasks []*types.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	return r.connMgr.WithRetry(ctx, func() error {
		pipe := r.client.Pipeline()

		for _, task := range tasks {
			if task.Queue == "" {
				task.Queue = "default"
			}

			// Serialize task
			taskData, err := r.serializer.Serialize(task)
			if err != nil {
				return fmt.Errorf("failed to serialize task %s: %w", task.ID, err)
			}

			// Add to stream
			streamName := r.config.GetStreamName(task.Queue)
			streamFields := map[string]interface{}{
				"task_id":    task.ID,
				"task_type":  string(task.Type),
				"priority":   string(task.Priority),
				"tenant_id":  task.TenantID,
				"created_at": task.CreatedAt.Unix(),
				"payload":    taskData,
			}

			pipe.XAdd(ctx, &redis.XAddArgs{
				Stream: streamName,
				MaxLen: r.config.MaxStreamLength,
				Approx: true,
				Values: streamFields,
			})

			// Add to priority queue
			priorityScore := r.calculatePriorityScore(task.Priority, task.CreatedAt)
			prioritySetName := r.config.GetPrioritySetName(task.Queue)
			pipe.ZAdd(ctx, prioritySetName, redis.Z{
				Score:  priorityScore,
				Member: task.ID,
			})

			// Store full task data
			taskHashKey := r.config.GetTaskHashKey(task.ID)
			pipe.HSet(ctx, taskHashKey, map[string]interface{}{
				"data":        taskData,
				"status":      string(task.Status),
				"enqueued_at": time.Now().Unix(),
			})
			pipe.Expire(ctx, taskHashKey, r.config.TaskTTL)
		}

		// Execute all operations atomically
		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to enqueue batch: %w", err)
		}

		r.logger.Info("batch enqueued successfully",
			types.Field{Key: "count", Value: len(tasks)},
		)

		return nil
	})
}

// DequeueBatch retrieves multiple tasks from the queue
func (r *Queue) DequeueBatch(ctx context.Context, queue string, count int, timeout time.Duration) ([]*types.Task, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be greater than 0")
	}

	if queue == "" {
		queue = "default"
	}

	var tasks []*types.Task

	for i := 0; i < count; i++ {
		// Use a shorter timeout for individual dequeue operations
		taskTimeout := timeout / time.Duration(count)
		if taskTimeout < 100*time.Millisecond {
			taskTimeout = 100 * time.Millisecond
		}

		task, err := r.Dequeue(ctx, queue, taskTimeout)
		if err != nil {
			// If we got some tasks, return them with the error
			if len(tasks) > 0 {
				return tasks, err
			}
			return nil, err
		}

		if task == nil {
			// No more tasks available
			break
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetTask retrieves a task by ID
func (r *Queue) GetTask(ctx context.Context, taskID string) (*types.Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task ID is required")
	}

	var task *types.Task
	err := r.connMgr.WithRetry(ctx, func() error {
		taskHashKey := r.config.GetTaskHashKey(taskID)

		result, err := r.client.HGetAll(ctx, taskHashKey).Result()
		if err != nil {
			return fmt.Errorf("failed to get task: %w", err)
		}

		if len(result) == 0 {
			task = nil
			return nil
		}

		taskData, exists := result["data"]
		if !exists {
			return fmt.Errorf("task data not found")
		}

		deserializedTask, err := r.serializer.Deserialize([]byte(taskData))
		if err != nil {
			return fmt.Errorf("failed to deserialize task: %w", err)
		}

		// Update status from hash if present
		if status, exists := result["status"]; exists {
			deserializedTask.Status = types.TaskStatus(status)
		}

		task = deserializedTask
		return nil
	})

	return task, err
}

// UpdateTask updates an existing task
func (r *Queue) UpdateTask(ctx context.Context, task *types.Task) error {
	if task == nil || task.ID == "" {
		return fmt.Errorf("task with ID is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		taskData, err := r.serializer.Serialize(task)
		if err != nil {
			return fmt.Errorf("failed to serialize task: %w", err)
		}

		taskHashKey := r.config.GetTaskHashKey(task.ID)

		updates := map[string]interface{}{
			"data":       taskData,
			"status":     string(task.Status),
			"updated_at": time.Now().Unix(),
		}

		err = r.client.HSet(ctx, taskHashKey, updates).Err()
		if err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}

		r.logger.Info("task updated successfully",
			types.Field{Key: "task_id", Value: task.ID},
		)

		return nil
	})
}

// DeleteTask removes a task completely
func (r *Queue) DeleteTask(ctx context.Context, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task ID is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		// Get task to find queue
		task, err := r.GetTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get task for deletion: %w", err)
		}

		if task == nil {
			return nil // Already deleted
		}

		pipe := r.client.Pipeline()

		// Remove from priority queue
		prioritySetName := r.config.GetPrioritySetName(task.Queue)
		pipe.ZRem(ctx, prioritySetName, taskID)

		// Remove task data
		taskHashKey := r.config.GetTaskHashKey(taskID)
		pipe.Del(ctx, taskHashKey)

		// Execute pipeline
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}

		r.logger.Info("task deleted successfully",
			types.Field{Key: "task_id", Value: taskID},
		)

		return nil
	})
}

// calculatePriorityScore calculates a score for priority ordering
// Higher priority tasks get lower scores (processed first)
func (r *Queue) calculatePriorityScore(priority types.Priority, createdAt time.Time) float64 {
	// Base priority scores (lower = higher priority)
	priorityScores := map[types.Priority]float64{
		types.PriorityCritical: 1000,
		types.PriorityHigh:     2000,
		types.PriorityNormal:   3000,
		types.PriorityLow:      4000,
	}

	baseScore, exists := priorityScores[priority]
	if !exists {
		baseScore = priorityScores[types.PriorityNormal]
	}

	// Add timestamp component for FIFO within same priority
	// Use milliseconds to ensure unique scores
	timestampScore := float64(createdAt.UnixMilli()) / 1000000.0

	return baseScore + timestampScore
}

// HealthCheck verifies the Redis connection and functionality
func (r *Queue) HealthCheck(ctx context.Context) error {
	return r.connMgr.HealthCheck(ctx)
}

// Close closes the Redis connection
func (r *Queue) Close() error {
	return r.connMgr.Close()
}
