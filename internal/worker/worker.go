package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// WorkerContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	WorkerIDKey ContextKey = "worker_id"
)

// Worker implements the types.Worker interface with enhanced functionality
// It processes tasks using the command pattern and reports to observers
type Worker struct {
	id              string
	config          *types.WorkerConfig
	queueBackend    types.QueueBackend
	commandRegistry *TaskCommandRegistry
	eventBus        *EventBus
	logger          types.Logger

	// State management
	state      types.WorkerStatus
	stateMutex sync.RWMutex

	// Task processing
	processors      map[types.TaskType]types.TaskProcessor
	processingTasks map[string]*types.Task
	taskMutex       sync.RWMutex

	// Lifecycle management
	ctx        context.Context
	cancel     context.CancelFunc
	shutdownCh chan struct{}
	wg         sync.WaitGroup

	// Metrics and monitoring
	startTime      time.Time
	lastHeartbeat  time.Time
	tasksProcessed int64
	tasksFailed    int64
	tasksCompleted int64

	// Worker metadata
	hostname     string
	capabilities []string
}

// NewWorker creates a new worker instance
func NewWorker(
	id string,
	config *types.WorkerConfig,
	queueBackend types.QueueBackend,
	commandRegistry *TaskCommandRegistry,
	eventBus *EventBus,
	logger types.Logger,
) *Worker {
	return &Worker{
		id:              id,
		config:          config,
		queueBackend:    queueBackend,
		commandRegistry: commandRegistry,
		eventBus:        eventBus,
		logger:          logger,
		state:           types.WorkerStatusIdle,
		processors:      make(map[types.TaskType]types.TaskProcessor),
		processingTasks: make(map[string]*types.Task),
		shutdownCh:      make(chan struct{}),
		capabilities:    make([]string, 0),
	}
}

// Start begins processing tasks from specified queues
func (w *Worker) Start(ctx context.Context, queues []string) error {
	w.stateMutex.Lock()
	if w.state != types.WorkerStatusIdle {
		w.stateMutex.Unlock()
		return fmt.Errorf("worker %s is already running", w.id)
	}

	w.state = types.WorkerStatusBusy
	w.startTime = time.Now()
	w.lastHeartbeat = time.Now()
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.stateMutex.Unlock()

	w.logger.Info("starting worker",
		types.Field{Key: "worker_id", Value: w.id},
		types.Field{Key: "queues", Value: queues})

	// Start task processing loops for each queue
	for _, queue := range queues {
		w.wg.Add(1)
		go w.taskProcessingLoop(w.ctx, queue)
	}

	// Start heartbeat loop
	w.wg.Add(1)
	go w.heartbeatLoop(w.ctx)

	// Wait for shutdown or context cancellation
	select {
	case <-w.shutdownCh:
		w.logger.Info("worker shutdown requested", types.Field{Key: "worker_id", Value: w.id})
	case <-ctx.Done():
		w.logger.Info("worker context cancelled", types.Field{Key: "worker_id", Value: w.id})
	}

	// Graceful shutdown
	w.cancel()
	w.wg.Wait()

	w.stateMutex.Lock()
	w.state = types.WorkerStatusOffline
	w.stateMutex.Unlock()

	w.logger.Info("worker stopped", types.Field{Key: "worker_id", Value: w.id})
	return nil
}

// Stop gracefully stops the worker, finishing current tasks
func (w *Worker) Stop(ctx context.Context) error {
	w.stateMutex.Lock()
	if w.state == types.WorkerStatusOffline {
		w.stateMutex.Unlock()
		return nil
	}

	w.state = types.WorkerStatusDraining
	w.stateMutex.Unlock()

	w.logger.Info("stopping worker gracefully", types.Field{Key: "worker_id", Value: w.id})

	// Signal shutdown
	close(w.shutdownCh)

	// Wait for graceful shutdown with timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.logger.Info("worker stopped gracefully", types.Field{Key: "worker_id", Value: w.id})
	case <-ctx.Done():
		w.logger.Warn("worker stop timeout", types.Field{Key: "worker_id", Value: w.id})
		w.cancel() // Force cancellation
		w.wg.Wait()
	}

	return nil
}

// RegisterProcessor adds a task processor for specific task types
func (w *Worker) RegisterProcessor(taskType types.TaskType, processor types.TaskProcessor) error {
	w.taskMutex.Lock()
	defer w.taskMutex.Unlock()

	if _, exists := w.processors[taskType]; exists {
		return fmt.Errorf("processor for task type %s already registered", taskType)
	}

	w.processors[taskType] = processor

	// Add to capabilities
	w.capabilities = append(w.capabilities, processor.GetCapabilities()...)

	w.logger.Info("task processor registered",
		types.Field{Key: "worker_id", Value: w.id},
		types.Field{Key: "task_type", Value: taskType},
		types.Field{Key: "capabilities", Value: processor.GetCapabilities()})

	return nil
}

// GetInfo returns current worker information
func (w *Worker) GetInfo() *types.WorkerInfo {
	w.stateMutex.RLock()
	state := w.state
	w.stateMutex.RUnlock()

	w.taskMutex.RLock()
	currentTasks := make([]string, 0, len(w.processingTasks))
	for taskID := range w.processingTasks {
		currentTasks = append(currentTasks, taskID)
	}
	capabilities := make([]string, len(w.capabilities))
	copy(capabilities, w.capabilities)
	w.taskMutex.RUnlock()

	return &types.WorkerInfo{
		ID:            w.id,
		Hostname:      w.hostname,
		Version:       "1.0.0", // Could be injected from build
		Queues:        w.config.Queues,
		Status:        state,
		RegisteredAt:  w.startTime,
		LastHeartbeat: w.lastHeartbeat,
		CurrentTasks:  currentTasks,
		Capabilities:  capabilities,
		Metadata: map[string]string{
			"tasks_processed": fmt.Sprintf("%d", w.tasksProcessed),
			"tasks_completed": fmt.Sprintf("%d", w.tasksCompleted),
			"tasks_failed":    fmt.Sprintf("%d", w.tasksFailed),
		},
	}
}

// Heartbeat updates the worker's status and metadata
func (w *Worker) Heartbeat(ctx context.Context) error {
	w.lastHeartbeat = time.Now()

	// Send heartbeat event
	w.eventBus.NotifyObservers(ctx, EventHealthy, &EventData{
		Event:          EventHealthy,
		WorkerID:       w.id,
		Timestamp:      w.lastHeartbeat,
		ActiveTasks:    len(w.processingTasks),
		CompletedTasks: w.tasksCompleted,
		FailedTasks:    w.tasksFailed,
	})

	return nil
}

// taskProcessingLoop processes tasks from a specific queue
func (w *Worker) taskProcessingLoop(ctx context.Context, queue string) {
	defer w.wg.Done()

	w.logger.Debug("starting task processing loop",
		types.Field{Key: "worker_id", Value: w.id},
		types.Field{Key: "queue", Value: queue})

	for {
		select {
		case <-ctx.Done():
			w.logger.Debug("task processing loop stopped",
				types.Field{Key: "worker_id", Value: w.id},
				types.Field{Key: "queue", Value: queue})
			return
		default:
			// Process next task
			if err := w.processNextTask(ctx, queue); err != nil {
				w.logger.Error("task processing error",
					types.Field{Key: "worker_id", Value: w.id},
					types.Field{Key: "queue", Value: queue},
					types.Field{Key: "error", Value: err.Error()})

				// Brief delay on error to prevent tight loop
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
					continue
				}
			}
		}
	}
}

// processNextTask dequeues and processes the next task from the queue
func (w *Worker) processNextTask(ctx context.Context, queue string) error {
	// Dequeue task with timeout
	task, err := w.queueBackend.Dequeue(ctx, queue, w.config.Timeout)
	if err != nil {
		// Timeout is expected when no tasks are available
		return nil
	}

	if task == nil {
		return nil // No task available
	}

	// Notify observers of task received
	w.eventBus.NotifyObservers(ctx, TaskEventReceived, &EventData{
		Event:     TaskEventReceived,
		WorkerID:  w.id,
		TaskID:    task.ID,
		TaskType:  task.Type,
		Queue:     queue,
		Timestamp: time.Now(),
	})

	// Add task to processing map
	w.taskMutex.Lock()
	w.processingTasks[task.ID] = task
	w.tasksProcessed++
	w.taskMutex.Unlock()

	// Process the task
	result := w.executeTask(ctx, task)

	// Remove from processing map
	w.taskMutex.Lock()
	delete(w.processingTasks, task.ID)
	if result.Status == types.TaskStatusCompleted {
		w.tasksCompleted++
	} else {
		w.tasksFailed++
	}
	w.taskMutex.Unlock()

	// Handle task completion
	return w.handleTaskResult(ctx, task, result)
}

// executeTask executes a task using the appropriate command
func (w *Worker) executeTask(ctx context.Context, task *types.Task) *types.TaskResult {
	startTime := time.Now()

	// Add worker ID to context
	taskCtx := context.WithValue(ctx, WorkerIDKey, w.id)

	// Set timeout if task has one
	if task.Timeout != nil {
		var cancel context.CancelFunc
		taskCtx, cancel = context.WithTimeout(taskCtx, *task.Timeout)
		defer cancel()
	}

	// Notify observers of task start
	w.eventBus.NotifyObservers(ctx, TaskEventStarted, &EventData{
		Event:     TaskEventStarted,
		WorkerID:  w.id,
		TaskID:    task.ID,
		TaskType:  task.Type,
		Queue:     task.Queue,
		Timestamp: startTime,
	})

	// Get command for this task
	command, err := w.commandRegistry.GetCommandForTask(task)
	if err != nil {
		duration := time.Since(startTime)

		w.eventBus.NotifyObservers(ctx, TaskEventFailed, &EventData{
			Event:     TaskEventFailed,
			WorkerID:  w.id,
			TaskID:    task.ID,
			TaskType:  task.Type,
			Queue:     task.Queue,
			Duration:  duration,
			Error:     err,
			Timestamp: time.Now(),
		})

		return &types.TaskResult{
			TaskID:      task.ID,
			Status:      types.TaskStatusFailed,
			Error:       fmt.Sprintf("no suitable command found: %v", err),
			Duration:    duration,
			CompletedAt: time.Now(),
			WorkerID:    w.id,
		}
	}

	// Execute the command
	result, err := command.Execute(taskCtx, task)
	duration := time.Since(startTime)

	// Ensure result has required fields
	if result == nil {
		result = &types.TaskResult{
			TaskID:   task.ID,
			WorkerID: w.id,
		}
	}

	result.Duration = duration
	result.CompletedAt = time.Now()
	result.WorkerID = w.id

	// Notify observers based on result
	if err != nil || result.Status == types.TaskStatusFailed {
		result.Status = types.TaskStatusFailed
		if err != nil && result.Error == "" {
			result.Error = err.Error()
		}

		w.eventBus.NotifyObservers(ctx, TaskEventFailed, &EventData{
			Event:     TaskEventFailed,
			WorkerID:  w.id,
			TaskID:    task.ID,
			TaskType:  task.Type,
			Queue:     task.Queue,
			Duration:  duration,
			Error:     err,
			Timestamp: time.Now(),
		})

		w.logger.Error("task execution failed",
			types.Field{Key: "worker_id", Value: w.id},
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "task_type", Value: task.Type},
			types.Field{Key: "duration", Value: duration},
			types.Field{Key: "error", Value: result.Error})
	} else {
		result.Status = types.TaskStatusCompleted

		w.eventBus.NotifyObservers(ctx, TaskEventCompleted, &EventData{
			Event:     TaskEventCompleted,
			WorkerID:  w.id,
			TaskID:    task.ID,
			TaskType:  task.Type,
			Queue:     task.Queue,
			Duration:  duration,
			Timestamp: time.Now(),
		})

		w.logger.Debug("task execution completed",
			types.Field{Key: "worker_id", Value: w.id},
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "task_type", Value: task.Type},
			types.Field{Key: "duration", Value: duration})
	}

	return result
}

// handleTaskResult handles the result of task execution
func (w *Worker) handleTaskResult(ctx context.Context, task *types.Task, result *types.TaskResult) error {
	switch result.Status {
	case types.TaskStatusCompleted:
		// Acknowledge successful completion
		if err := w.queueBackend.Ack(ctx, task.ID); err != nil {
			w.logger.Error("failed to ack completed task",
				types.Field{Key: "task_id", Value: task.ID},
				types.Field{Key: "error", Value: err.Error()})
			return err
		}

	case types.TaskStatusFailed:
		// Handle failed task based on retry policy
		return w.handleFailedTask(ctx, task, result)

	default:
		w.logger.Warn("unexpected task result status",
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "status", Value: result.Status})
	}

	return nil
}

// handleFailedTask handles retry logic for failed tasks
func (w *Worker) handleFailedTask(ctx context.Context, task *types.Task, result *types.TaskResult) error {
	// Check if task has retries remaining
	if task.CurrentRetries >= task.MaxRetries {
		// Move to dead letter queue
		w.logger.Warn("task exhausted retries, moving to DLQ",
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "retries", Value: task.CurrentRetries},
			types.Field{Key: "max_retries", Value: task.MaxRetries})

		if err := w.queueBackend.MoveToDLQ(ctx, task.ID, result.Error); err != nil {
			w.logger.Error("failed to move task to DLQ",
				types.Field{Key: "task_id", Value: task.ID},
				types.Field{Key: "error", Value: err.Error()})
			return err
		}

		return nil
	}

	// Schedule retry
	retryAt := w.calculateNextRetry(task.CurrentRetries)
	if err := w.queueBackend.ScheduleRetry(ctx, task.ID, retryAt); err != nil {
		w.logger.Error("failed to schedule retry",
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "retry_at", Value: retryAt},
			types.Field{Key: "error", Value: err.Error()})
		return err
	}

	// Notify observers of retry
	w.eventBus.NotifyObservers(ctx, TaskEventRetrying, &EventData{
		Event:     TaskEventRetrying,
		WorkerID:  w.id,
		TaskID:    task.ID,
		TaskType:  task.Type,
		Queue:     task.Queue,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"retry_attempt": task.CurrentRetries + 1,
			"retry_at":      retryAt,
			"reason":        result.Error,
		},
	})

	w.logger.Info("task scheduled for retry",
		types.Field{Key: "task_id", Value: task.ID},
		types.Field{Key: "retry_attempt", Value: task.CurrentRetries + 1},
		types.Field{Key: "retry_at", Value: retryAt})

	// NACK the current task
	return w.queueBackend.Nack(ctx, task.ID, result.Error)
}

// calculateNextRetry calculates the next retry time using exponential backoff
func (w *Worker) calculateNextRetry(currentRetries int) time.Time {
	// Use exponential backoff with jitter
	delay := w.config.InitialDelay

	// Exponential backoff
	for i := 0; i < currentRetries; i++ {
		delay = time.Duration(float64(delay) * w.config.BackoffFactor)
		if delay > w.config.MaxDelay {
			delay = w.config.MaxDelay
			break
		}
	}

	// Add jitter (up to 25% of delay)
	jitter := time.Duration(float64(delay) * 0.25 * (0.5 + (float64(time.Now().UnixNano()%1000) / 1000)))

	return time.Now().Add(delay + jitter)
}

// heartbeatLoop sends regular heartbeat updates
func (w *Worker) heartbeatLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Heartbeat(ctx); err != nil {
				w.logger.Error("heartbeat failed",
					types.Field{Key: "worker_id", Value: w.id},
					types.Field{Key: "error", Value: err.Error()})
			}
		}
	}
}
