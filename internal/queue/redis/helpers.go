package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
	"github.com/redis/go-redis/v9"
)

// initializeConsumerGroups creates consumer groups for the specified queues
func (r *Queue) initializeConsumerGroups(ctx context.Context) error {
	// Default queues to initialize
	defaultQueues := []string{"default", "high_priority", "low_priority"}

	for _, queue := range defaultQueues {
		if err := r.ensureConsumerGroup(ctx, queue); err != nil {
			r.logger.Warn("failed to initialize consumer group",
				types.Field{Key: "queue", Value: queue},
				types.Field{Key: "error", Value: err},
			)
		}
	}

	return nil
}

// ensureConsumerGroup creates a consumer group if it doesn't exist
func (r *Queue) ensureConsumerGroup(ctx context.Context, queue string) error {
	streamName := r.config.GetStreamName(queue)

	// Try to create the consumer group
	err := r.client.XGroupCreate(ctx, streamName, r.config.ConsumerGroup, "0").Err()
	if err != nil {
		// Check if error is because group already exists
		if err.Error() == "BUSYGROUP Consumer Group name already exists" {
			return nil // Group already exists, which is fine
		}

		// Check if error is because stream doesn't exist
		if err.Error() == "ERR The XGROUP subcommand requires the key to exist" {
			// Create an empty stream first
			_, err = r.client.XAdd(ctx, &redis.XAddArgs{
				Stream: streamName,
				Values: map[string]interface{}{"_": "_"},
			}).Result()
			if err != nil {
				return fmt.Errorf("failed to create stream: %w", err)
			}

			// Delete the dummy entry
			entries, err := r.client.XRange(ctx, streamName, "-", "+").Result()
			if err == nil && len(entries) > 0 {
				r.client.XDel(ctx, streamName, entries[0].ID)
			}

			// Now try to create the group again
			err = r.client.XGroupCreate(ctx, streamName, r.config.ConsumerGroup, "0").Err()
			if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
				return fmt.Errorf("failed to create consumer group after creating stream: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create consumer group: %w", err)
		}
	}

	return nil
}

// claimPendingMessage attempts to claim pending messages from other consumers
func (r *Queue) claimPendingMessage(ctx context.Context, queue string) (*types.Task, error) {
	streamName := r.config.GetStreamName(queue)

	// Get pending messages for the consumer group
	pending, err := r.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: streamName,
		Group:  r.config.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  1,
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get pending messages: %w", err)
	}

	if len(pending) == 0 {
		return nil, nil // No pending messages
	}

	// Check if the message has been idle long enough to claim
	message := pending[0]
	if message.Idle < r.config.ClaimMinIdleTime {
		return nil, nil // Not idle long enough
	}

	// Claim the message
	consumerName := r.config.GetConsumerName("claimer")
	claimed, err := r.client.XClaim(ctx, &redis.XClaimArgs{
		Stream:   streamName,
		Group:    r.config.ConsumerGroup,
		Consumer: consumerName,
		MinIdle:  r.config.ClaimMinIdleTime,
		Messages: []string{message.ID},
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to claim message: %w", err)
	}

	if len(claimed) == 0 {
		return nil, nil // Message was claimed by another consumer
	}

	// Parse the claimed message
	entry := claimed[0]
	return r.parseStreamEntry(ctx, entry)
}

// readNewMessage reads a new message from the stream
func (r *Queue) readNewMessage(ctx context.Context, queue string, timeout time.Duration) (*types.Task, error) {
	streamName := r.config.GetStreamName(queue)
	consumerName := r.config.GetConsumerName("reader")

	// Use XREADGROUP to read from the stream
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    r.config.ConsumerGroup,
		Consumer: consumerName,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    timeout,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil, nil // No messages available
		}
		return nil, fmt.Errorf("failed to read from stream: %w", err)
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil, nil // No messages available
	}

	// Parse the message
	entry := streams[0].Messages[0]
	return r.parseStreamEntry(ctx, entry)
}

// parseStreamEntry parses a Redis stream entry into a task
func (r *Queue) parseStreamEntry(ctx context.Context, entry redis.XMessage) (*types.Task, error) {
	taskID, exists := entry.Values["task_id"]
	if !exists {
		return nil, fmt.Errorf("task_id not found in stream entry")
	}

	taskIDStr, ok := taskID.(string)
	if !ok {
		return nil, fmt.Errorf("task_id is not a string")
	}

	// Get the full task data from the hash
	task, err := r.GetTask(ctx, taskIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get task data: %w", err)
	}

	if task == nil {
		return nil, fmt.Errorf("task data not found for ID: %s", taskIDStr)
	}

	// Update task status to running
	task.Status = types.TaskStatusRunning
	task.StartedAt = timePtr(time.Now())

	// Update the task in storage
	if err := r.UpdateTask(ctx, task); err != nil {
		r.logger.Warn("failed to update task status to running",
			types.Field{Key: "task_id", Value: taskIDStr},
			types.Field{Key: "error", Value: err},
		)
	}

	return task, nil
}

// findStreamEntryID finds the stream entry ID for a specific task
func (r *Queue) findStreamEntryID(ctx context.Context, streamName, taskID string) (string, error) {
	// Search through recent stream entries to find the one with matching task_id
	entries, err := r.client.XRevRange(ctx, streamName, "+", "-").Result()
	if err != nil {
		return "", fmt.Errorf("failed to search stream entries: %w", err)
	}

	for _, entry := range entries {
		if entryTaskID, exists := entry.Values["task_id"]; exists {
			if entryTaskID == taskID {
				return entry.ID, nil
			}
		}
	}

	return "", nil // Entry not found
}

// scheduleRetry schedules a task for retry
func (r *Queue) scheduleRetry(ctx context.Context, task *types.Task, reason string) error {
	// Calculate next retry time
	nextRetry := r.calculateNextRetryTime(task)

	// Update task for retry
	task.CurrentRetries++
	task.LastError = reason
	task.NextRetryAt = &nextRetry
	task.Status = types.TaskStatusPending

	// If retry time is in the future, schedule it
	if nextRetry.After(time.Now()) {
		return r.ScheduleRetry(ctx, task.ID, nextRetry)
	}

	// Otherwise, re-enqueue immediately
	return r.Enqueue(ctx, task)
}

// calculateNextRetryTime calculates when to retry a task next
func (r *Queue) calculateNextRetryTime(task *types.Task) time.Time {
	// Exponential backoff: 1s, 2s, 4s, 8s, etc. up to max of 5 minutes
	delay := time.Duration(1<<uint(task.CurrentRetries)) * time.Second
	maxDelay := 5 * time.Minute

	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter to prevent thundering herd
	jitter := time.Duration(task.CurrentRetries*100) * time.Millisecond

	return time.Now().Add(delay + jitter)
}

// timePtr returns a pointer to the given time
func timePtr(t time.Time) *time.Time {
	return &t
}

// GetQueueStats returns statistics for a specific queue
func (r *Queue) GetQueueStats(ctx context.Context, queue string) (*types.QueueStats, error) {
	if queue == "" {
		queue = "default"
	}

	var stats *types.QueueStats
	err := r.connMgr.WithRetry(ctx, func() error {
		streamName := r.config.GetStreamName(queue)
		prioritySetName := r.config.GetPrioritySetName(queue)

		pipe := r.client.Pipeline()

		// Get stream info
		streamInfoCmd := pipe.XInfoStream(ctx, streamName)

		// Get priority set size
		priorityCountCmd := pipe.ZCard(ctx, prioritySetName)

		// Execute pipeline
		_, err := pipe.Exec(ctx)
		if err != nil && err != redis.Nil {
			return fmt.Errorf("failed to get queue stats: %w", err)
		}

		streamInfo, err := streamInfoCmd.Result()
		pendingTasks := int64(0)
		if err == nil {
			pendingTasks = streamInfo.Length
		}

		priorityCount, err := priorityCountCmd.Result()
		if err != nil && err != redis.Nil {
			return fmt.Errorf("failed to get priority count: %w", err)
		}

		stats = &types.QueueStats{
			QueueName:    queue,
			PendingTasks: pendingTasks,
			RunningTasks: 0, // Would need additional tracking
			TasksByPriority: map[types.Priority]int64{
				types.PriorityCritical: 0,
				types.PriorityHigh:     0,
				types.PriorityNormal:   priorityCount,
				types.PriorityLow:      0,
			},
			TasksByType: map[types.TaskType]int64{},
			LastUpdated: time.Now(),
		}

		return nil
	})

	return stats, err
}

// ListQueues returns a list of all known queues
func (r *Queue) ListQueues(ctx context.Context) ([]string, error) {
	var queues []string
	err := r.connMgr.WithRetry(ctx, func() error {
		// Search for all stream keys with our prefix
		pattern := r.config.StreamPrefix + "*"
		keys, err := r.client.Keys(ctx, pattern).Result()
		if err != nil {
			return fmt.Errorf("failed to list queues: %w", err)
		}

		// Extract queue names from stream keys
		queueSet := make(map[string]bool)
		for _, key := range keys {
			if len(key) > len(r.config.StreamPrefix) {
				queueName := key[len(r.config.StreamPrefix):]
				// Skip DLQ streams
				if !strings.Contains(queueName, r.config.DLQSuffix) {
					queueSet[queueName] = true
				}
			}
		}

		// Convert to slice
		queues = make([]string, 0, len(queueSet))
		for queue := range queueSet {
			queues = append(queues, queue)
		}

		return nil
	})

	return queues, err
}

// PurgeQueue removes all tasks from a queue
func (r *Queue) PurgeQueue(ctx context.Context, queue string) error {
	if queue == "" {
		return fmt.Errorf("queue name is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		streamName := r.config.GetStreamName(queue)
		prioritySetName := r.config.GetPrioritySetName(queue)

		pipe := r.client.Pipeline()

		// Delete the stream
		pipe.Del(ctx, streamName)

		// Delete the priority set
		pipe.Del(ctx, prioritySetName)

		// Execute pipeline
		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to purge queue: %w", err)
		}

		r.logger.Info("queue purged successfully",
			types.Field{Key: "queue", Value: queue},
		)

		return nil
	})
}

// MoveToDLQ moves a task to the dead letter queue
func (r *Queue) MoveToDLQ(ctx context.Context, taskID string, reason string) error {
	if taskID == "" {
		return fmt.Errorf("task ID is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		// Get the task
		task, err := r.GetTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get task for DLQ: %w", err)
		}

		if task == nil {
			return fmt.Errorf("task not found: %s", taskID)
		}

		// Update task status
		task.Status = types.TaskStatusDeadLetter
		task.LastError = reason

		// Serialize task for DLQ
		taskData, err := r.serializer.Serialize(task)
		if err != nil {
			return fmt.Errorf("failed to serialize task for DLQ: %w", err)
		}

		dlqStreamName := r.config.GetDLQStreamName(task.Queue)

		pipe := r.client.Pipeline()

		// Add to DLQ stream
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: dlqStreamName,
			MaxLen: r.config.DLQMaxEntries,
			Approx: true,
			Values: map[string]interface{}{
				"task_id":     taskID,
				"reason":      reason,
				"moved_at":    time.Now().Unix(),
				"task_data":   taskData,
				"retry_count": task.CurrentRetries,
			},
		})

		// Remove from priority queue
		prioritySetName := r.config.GetPrioritySetName(task.Queue)
		pipe.ZRem(ctx, prioritySetName, taskID)

		// Update task status
		taskHashKey := r.config.GetTaskHashKey(taskID)
		pipe.HSet(ctx, taskHashKey, map[string]interface{}{
			"status":          string(types.TaskStatusDeadLetter),
			"last_error":      reason,
			"moved_to_dlq_at": time.Now().Unix(),
		})

		// Execute pipeline
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to move task to DLQ: %w", err)
		}

		r.logger.Info("task moved to dead letter queue",
			types.Field{Key: "task_id", Value: taskID},
			types.Field{Key: "queue", Value: task.Queue},
			types.Field{Key: "reason", Value: reason},
		)

		return nil
	})
}

// RequeueFromDLQ moves a task back from dead letter queue to normal queue
func (r *Queue) RequeueFromDLQ(ctx context.Context, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task ID is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		// Get the task
		task, err := r.GetTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get task for requeue: %w", err)
		}

		if task == nil {
			return fmt.Errorf("task not found: %s", taskID)
		}

		if task.Status != types.TaskStatusDeadLetter {
			return fmt.Errorf("task is not in dead letter queue: %s", taskID)
		}

		// Reset task for requeue
		task.Status = types.TaskStatusPending
		task.CurrentRetries = 0
		task.LastError = ""
		task.NextRetryAt = nil

		// Re-enqueue the task
		return r.Enqueue(ctx, task)
	})
}

// ScheduleRetry schedules a task for retry at a specific time
func (r *Queue) ScheduleRetry(ctx context.Context, taskID string, retryAt time.Time) error {
	if taskID == "" {
		return fmt.Errorf("task ID is required")
	}

	return r.connMgr.WithRetry(ctx, func() error {
		// For now, we'll implement a simple approach
		// In a production system, you might use a separate scheduled tasks system

		// Update the task's retry time
		taskHashKey := r.config.GetTaskHashKey(taskID)

		err := r.client.HSet(ctx, taskHashKey, map[string]interface{}{
			"next_retry_at": retryAt.Unix(),
			"status":        string(types.TaskStatusPending),
		}).Err()

		if err != nil {
			return fmt.Errorf("failed to schedule retry: %w", err)
		}

		r.logger.Info("task scheduled for retry",
			types.Field{Key: "task_id", Value: taskID},
			types.Field{Key: "retry_at", Value: retryAt},
		)

		return nil
	})
}

// GetScheduledTasks returns tasks scheduled to run before the specified time
func (r *Queue) GetScheduledTasks(ctx context.Context, before time.Time, limit int) ([]*types.Task, error) {
	// This is a simplified implementation
	// In production, you'd want a more sophisticated scheduling system
	var tasks []*types.Task

	err := r.connMgr.WithRetry(ctx, func() error {
		// Search for tasks with retry times before the specified time
		pattern := r.config.TaskHashPrefix + "*"
		keys, err := r.client.Keys(ctx, pattern).Result()
		if err != nil {
			return fmt.Errorf("failed to get scheduled tasks: %w", err)
		}

		count := 0
		for _, key := range keys {
			if count >= limit {
				break
			}

			retryAtStr, err := r.client.HGet(ctx, key, "next_retry_at").Result()
			if err == redis.Nil {
				continue // No retry time set
			}
			if err != nil {
				continue // Skip on error
			}

			retryAt, err := strconv.ParseInt(retryAtStr, 10, 64)
			if err != nil {
				continue // Skip invalid retry time
			}

			if time.Unix(retryAt, 0).Before(before) {
				taskID := key[len(r.config.TaskHashPrefix):]
				task, err := r.GetTask(ctx, taskID)
				if err != nil {
					continue // Skip on error
				}
				if task != nil {
					tasks = append(tasks, task)
					count++
				}
			}
		}

		return nil
	})

	return tasks, err
}
