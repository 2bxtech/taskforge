package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
	rds "github.com/redis/go-redis/v9"
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
			_, err = r.client.XAdd(ctx, &rds.XAddArgs{
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
	pending, err := r.client.XPendingExt(ctx, &rds.XPendingExtArgs{
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
	claimed, err := r.client.XClaim(ctx, &rds.XClaimArgs{
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
	streams, err := r.client.XReadGroup(ctx, &rds.XReadGroupArgs{
		Group:    r.config.ConsumerGroup,
		Consumer: consumerName,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    timeout,
	}).Result()

	if err != nil {
		if err == rds.Nil {
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
func (r *Queue) parseStreamEntry(ctx context.Context, entry rds.XMessage) (*types.Task, error) {
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

	if err := r.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to persist retry state: %w", err)
	}

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
	// Ensure CurrentRetries is non-negative and within reasonable bounds to prevent integer overflow
	retries := task.CurrentRetries
	if retries < 0 {
		retries = 0
	}
	// Cap retries at 20 to prevent excessive delays and potential overflow (2^20 = ~1M seconds)
	const maxRetries = 20
	if retries > maxRetries {
		retries = maxRetries
	}

	// Use int64 explicitly to avoid any potential overflow issues
	// This addresses gosec G115 warning about integer overflow conversion
	delay := time.Duration(1<<uint(retries)) * time.Second // Safe: retries is bounded 0-20

	const maxDelay = 5 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter to prevent thundering herd
	// Using bounded calculation to ensure no overflow
	jitter := time.Duration(min(retries*100, 5000)) * time.Millisecond

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
		if err != nil && err != rds.Nil {
			return fmt.Errorf("failed to get queue stats: %w", err)
		}

		streamInfo, err := streamInfoCmd.Result()
		pendingTasks := int64(0)
		if err == nil {
			pendingTasks = streamInfo.Length
		}

		// Get the priority count result
		priorityCount, err := priorityCountCmd.Result()
		if err != nil && err != rds.Nil {
			// Log the error but don't fail the entire operation
			r.logger.Warn("failed to get priority count",
				types.Field{Key: "queue", Value: queue},
				types.Field{Key: "error", Value: err},
			)
			priorityCount = 0
		}

		// Create stats with actual priority count
		stats = &types.QueueStats{
			QueueName:    queue,
			PendingTasks: pendingTasks,
			RunningTasks: 0, // Would need additional tracking
			TasksByPriority: map[types.Priority]int64{
				// This is simplified - in larger systems you'd want to
				// track tasks by actual priority levels
				types.PriorityNormal: priorityCount,
			},
			TasksByType: map[types.TaskType]int64{},
			LastUpdated: time.Now(),
		}

		// Log the priority count for debugging
		r.logger.Debug("queue stats retrieved",
			types.Field{Key: "queue", Value: queue},
			types.Field{Key: "pending_tasks", Value: pendingTasks},
			types.Field{Key: "priority_count", Value: priorityCount},
		)

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
		pipe.XAdd(ctx, &rds.XAddArgs{
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

		// Acknowledge the delivery that exhausted its retries so it cannot be
		// reclaimed from the consumer group's pending-entry list.
		streamName := r.config.GetStreamName(task.Queue)
		entryID, findErr := r.findStreamEntryID(ctx, streamName, taskID)
		if findErr != nil {
			return fmt.Errorf("failed to find stream entry for DLQ: %w", findErr)
		}
		if entryID != "" {
			pipe.XAck(ctx, streamName, r.config.ConsumerGroup, entryID)
		}

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
		task, err := r.GetTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get task for retry: %w", err)
		}
		if task == nil {
			return fmt.Errorf("task not found: %s", taskID)
		}

		taskHashKey := r.config.GetTaskHashKey(taskID)
		streamName := r.config.GetStreamName(task.Queue)
		entryID, err := r.findStreamEntryID(ctx, streamName, taskID)
		if err != nil {
			return fmt.Errorf("failed to find stream entry for retry: %w", err)
		}

		pipe := r.client.TxPipeline()
		pipe.HSet(ctx, taskHashKey, map[string]interface{}{
			"next_retry_at": retryAt.Unix(),
			"status":        string(types.TaskStatusPending),
		})
		pipe.ZAdd(ctx, r.config.ScheduledSetName, rds.Z{
			Score:  float64(retryAt.UnixMilli()),
			Member: taskID,
		})
		pipe.ZRem(ctx, r.config.GetPrioritySetName(task.Queue), taskID)
		if entryID != "" {
			pipe.XAck(ctx, streamName, r.config.ConsumerGroup, entryID)
		}
		if _, err := pipe.Exec(ctx); err != nil {
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
	// Use sorted set for efficient scheduled task retrieval
	// This replaces the inefficient KEYS pattern scan approach
	var tasks []*types.Task

	err := r.connMgr.WithRetry(ctx, func() error {
		// Use ZRANGEBYSCORE to efficiently get tasks scheduled before 'before' time
		// In a sorted set, task IDs are members and next_retry_at timestamps are scores
		// Get task IDs with scores (timestamps) less than the 'before' time
		results, err := r.client.ZRangeByScoreWithScores(ctx, r.config.ScheduledSetName, &rds.ZRangeBy{
			Min:    "0",
			Max:    fmt.Sprintf("%d", before.UnixMilli()),
			Offset: 0,
			Count:  int64(limit),
		}).Result()

		if err != nil && err != rds.Nil {
			return fmt.Errorf("failed to get scheduled tasks from sorted set: %w", err)
		}

		// Fetch the actual task data for each returned task ID
		for _, result := range results {
			taskID := result.Member.(string)
			task, err := r.GetTask(ctx, taskID)
			if err != nil {
				// Log error but continue processing other tasks
				continue
			}
			if task != nil {
				tasks = append(tasks, task)
			}
		}

		return nil
	})

	return tasks, err
}

// promoteScheduledRetries atomically removes due retries from the schedule and
// appends their replacement delivery. Concurrent workers cannot promote the
// same scheduled member, and Redis cannot observe a removed-but-not-enqueued
// intermediate state.
func (r *Queue) promoteScheduledRetries(ctx context.Context, queue string, before time.Time, limit int64) error {
	results, err := r.client.ZRangeByScore(ctx, r.config.ScheduledSetName, &rds.ZRangeBy{
		Min:   "0",
		Max:   fmt.Sprintf("%d", before.UnixMilli()),
		Count: limit,
	}).Result()
	if err != nil && err != rds.Nil {
		return err
	}

	for _, taskID := range results {
		task, err := r.GetTask(ctx, taskID)
		if err != nil {
			return err
		}
		if task == nil {
			r.client.ZRem(ctx, r.config.ScheduledSetName, taskID)
			continue
		}
		if task.Queue != queue {
			continue
		}

		task.NextRetryAt = nil
		promoted, err := r.promoteScheduledTask(ctx, task)
		if err != nil {
			return err
		}
		if promoted {
			r.logger.Info("scheduled retry promoted",
				types.Field{Key: "task_id", Value: task.ID},
				types.Field{Key: "queue", Value: task.Queue},
			)
		}
	}
	return nil
}

func (r *Queue) promoteScheduledTask(ctx context.Context, task *types.Task) (bool, error) {
	taskData, err := r.serializer.Serialize(task)
	if err != nil {
		return false, fmt.Errorf("failed to serialize scheduled retry: %w", err)
	}

	const promoteScript = `
if redis.call("ZSCORE", KEYS[1], ARGV[1]) == false then
	return 0
end
redis.call("ZREM", KEYS[1], ARGV[1])
redis.call("XADD", KEYS[2], "MAXLEN", "~", ARGV[2], "*",
	"task_id", ARGV[1],
	"task_type", ARGV[3],
	"priority", ARGV[4],
	"tenant_id", ARGV[5],
	"created_at", ARGV[6],
	"payload", ARGV[7])
redis.call("ZADD", KEYS[3], ARGV[8], ARGV[1])
redis.call("HSET", KEYS[4],
	"data", ARGV[7],
	"status", ARGV[9],
	"enqueued_at", ARGV[10])
redis.call("PEXPIRE", KEYS[4], ARGV[11])
return 1
`

	result, err := r.client.Eval(ctx, promoteScript, []string{
		r.config.ScheduledSetName,
		r.config.GetStreamName(task.Queue),
		r.config.GetPrioritySetName(task.Queue),
		r.config.GetTaskHashKey(task.ID),
	},
		task.ID,
		r.config.MaxStreamLength,
		string(task.Type),
		string(task.Priority),
		task.TenantID,
		task.CreatedAt.Unix(),
		taskData,
		r.calculatePriorityScore(task.Priority, task.CreatedAt),
		string(types.TaskStatusPending),
		time.Now().Unix(),
		r.config.TaskTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to promote scheduled retry: %w", err)
	}
	return result == 1, nil
}
