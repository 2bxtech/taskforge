package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/2bxtech/taskforge/internal/queue/factory"
	"github.com/2bxtech/taskforge/internal/worker"
	"github.com/2bxtech/taskforge/pkg/types"
)

// ExampleLogger implements the types.Logger interface for demo purposes
type ExampleLogger struct{}

func (l *ExampleLogger) Debug(msg string, fields ...types.Field) {
	log.Printf("[DEBUG] %s %s", msg, formatFields(fields))
}

func (l *ExampleLogger) Info(msg string, fields ...types.Field) {
	log.Printf("[INFO] %s %s", msg, formatFields(fields))
}

func (l *ExampleLogger) Warn(msg string, fields ...types.Field) {
	log.Printf("[WARN] %s %s", msg, formatFields(fields))
}

func (l *ExampleLogger) Error(msg string, fields ...types.Field) {
	log.Printf("[ERROR] %s %s", msg, formatFields(fields))
}

func (l *ExampleLogger) With(fields ...types.Field) types.Logger {
	return l // Simple implementation for demo
}

func formatFields(fields []types.Field) string {
	if len(fields) == 0 {
		return ""
	}

	result := "["
	for i, field := range fields {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%s=%v", field.Key, field.Value)
	}
	result += "]"
	return result
}

// ExampleMetricsCollector implements types.MetricsCollector for demo
type ExampleMetricsCollector struct {
	logger types.Logger
}

func NewExampleMetricsCollector(logger types.Logger) *ExampleMetricsCollector {
	return &ExampleMetricsCollector{logger: logger}
}

func (m *ExampleMetricsCollector) RecordTaskEnqueued(taskType types.TaskType, priority types.Priority, queue string) {
	m.logger.Info("task enqueued",
		types.Field{Key: "task_type", Value: taskType},
		types.Field{Key: "priority", Value: priority},
		types.Field{Key: "queue", Value: queue})
}

func (m *ExampleMetricsCollector) RecordTaskStarted(taskType types.TaskType, priority types.Priority, queue string) {
	m.logger.Info("task started",
		types.Field{Key: "task_type", Value: taskType},
		types.Field{Key: "priority", Value: priority},
		types.Field{Key: "queue", Value: queue})
}

func (m *ExampleMetricsCollector) RecordTaskCompleted(taskType types.TaskType, priority types.Priority, queue string, duration time.Duration) {
	m.logger.Info("task completed",
		types.Field{Key: "task_type", Value: taskType},
		types.Field{Key: "priority", Value: priority},
		types.Field{Key: "queue", Value: queue},
		types.Field{Key: "duration", Value: duration})
}

func (m *ExampleMetricsCollector) RecordTaskFailed(taskType types.TaskType, priority types.Priority, queue string, duration time.Duration, errorType string) {
	m.logger.Error("task failed",
		types.Field{Key: "task_type", Value: taskType},
		types.Field{Key: "priority", Value: priority},
		types.Field{Key: "queue", Value: queue},
		types.Field{Key: "duration", Value: duration},
		types.Field{Key: "error_type", Value: errorType})
}

func (m *ExampleMetricsCollector) UpdateQueueDepth(queue string, depth int64) {
	m.logger.Debug("queue depth updated", types.Field{Key: "queue", Value: queue}, types.Field{Key: "depth", Value: depth})
}

func (m *ExampleMetricsCollector) UpdateActiveWorkers(queue string, count int64) {
	m.logger.Debug("active workers updated", types.Field{Key: "queue", Value: queue}, types.Field{Key: "count", Value: count})
}

func (m *ExampleMetricsCollector) RecordWorkerRegistered(workerID string, queues []string) {
	m.logger.Info("worker registered", types.Field{Key: "worker_id", Value: workerID}, types.Field{Key: "queues", Value: queues})
}

func (m *ExampleMetricsCollector) RecordWorkerUnregistered(workerID string) {
	m.logger.Info("worker unregistered", types.Field{Key: "worker_id", Value: workerID})
}

func (m *ExampleMetricsCollector) UpdateWorkerStatus(workerID string, status types.WorkerStatus) {
	m.logger.Debug("worker status updated", types.Field{Key: "worker_id", Value: workerID}, types.Field{Key: "status", Value: status})
}

func (m *ExampleMetricsCollector) RecordCircuitBreakerOpen(service string) {
	m.logger.Warn("circuit breaker opened", types.Field{Key: "service", Value: service})
}

func (m *ExampleMetricsCollector) RecordCircuitBreakerClosed(service string) {
	m.logger.Info("circuit breaker closed", types.Field{Key: "service", Value: service})
}

func (m *ExampleMetricsCollector) RecordCircuitBreakerHalfOpen(service string) {
	m.logger.Info("circuit breaker half-open", types.Field{Key: "service", Value: service})
}

// WebhookProcessor implements types.TaskProcessor for webhook tasks
type WebhookProcessor struct {
	logger types.Logger
}

func NewWebhookProcessor(logger types.Logger) *WebhookProcessor {
	return &WebhookProcessor{logger: logger}
}

func (w *WebhookProcessor) Process(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
	// Simulate webhook processing
	w.logger.Info("processing webhook task", types.Field{Key: "task_id", Value: task.ID})

	// Parse webhook payload
	var payload map[string]interface{}
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return nil, fmt.Errorf("invalid webhook payload: %w", err)
	}

	// Simulate processing time
	time.Sleep(100 * time.Millisecond)

	// Simulate occasional failure (10% chance)
	if task.ID[len(task.ID)-1] < '2' { // Simple failure simulation based on ID
		return nil, fmt.Errorf("webhook delivery failed: connection timeout")
	}

	result := map[string]interface{}{
		"status":      "delivered",
		"status_code": 200,
		"response":    "webhook delivered successfully",
	}

	resultBytes, _ := json.Marshal(result)
	return &types.TaskResult{
		TaskID: task.ID,
		Status: types.TaskStatusCompleted,
		Result: resultBytes,
	}, nil
}

func (w *WebhookProcessor) GetSupportedTypes() []types.TaskType {
	return []types.TaskType{types.TaskTypeWebhook}
}

func (w *WebhookProcessor) GetCapabilities() []string {
	return []string{"http-webhooks", "retry-logic"}
}

// EmailProcessor implements types.TaskProcessor for email tasks
type EmailProcessor struct {
	logger types.Logger
}

func NewEmailProcessor(logger types.Logger) *EmailProcessor {
	return &EmailProcessor{logger: logger}
}

func (e *EmailProcessor) Process(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
	e.logger.Info("processing email task", types.Field{Key: "task_id", Value: task.ID})

	// Parse email payload
	var payload map[string]interface{}
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return nil, fmt.Errorf("invalid email payload: %w", err)
	}

	// Simulate processing time
	time.Sleep(200 * time.Millisecond)

	// Simulate occasional failure (5% chance)
	if task.ID[len(task.ID)-1] < '1' {
		return nil, fmt.Errorf("email sending failed: SMTP error")
	}

	result := map[string]interface{}{
		"status":     "sent",
		"message_id": fmt.Sprintf("msg_%s", task.ID),
	}

	resultBytes, _ := json.Marshal(result)
	return &types.TaskResult{
		TaskID: task.ID,
		Status: types.TaskStatusCompleted,
		Result: resultBytes,
	}, nil
}

func (e *EmailProcessor) GetSupportedTypes() []types.TaskType {
	return []types.TaskType{types.TaskTypeEmail}
}

func (e *EmailProcessor) GetCapabilities() []string {
	return []string{"smtp", "templating"}
}

func main() {
	logger := &ExampleLogger{}
	logger.Info("TaskForge Phase 2B Worker Engine Demo")

	// Create configuration
	config := types.DefaultConfig()
	config.Worker.Concurrency = 3
	config.Worker.Queues = []string{"webhooks", "emails", "default"}
	config.Worker.HeartbeatInterval = 5 * time.Second
	config.Worker.ShutdownTimeout = 30 * time.Second

	// Create queue backend using Phase 2A factory
	queueFactory := factory.NewQueueBackendFactory()
	queueBackend, err := queueFactory.CreateQueueBackend(&config.Queue, logger)
	if err != nil {
		log.Fatalf("Failed to create queue backend: %v", err)
	}
	defer queueBackend.Close()

	// Create metrics collector
	metricsCollector := NewExampleMetricsCollector(logger)

	// Create worker pool
	workerPool, err := worker.NewWorkerPool(
		"demo-worker-pool",
		&config.Worker,
		queueBackend,
		metricsCollector,
		logger,
	)
	if err != nil {
		log.Fatalf("Failed to create worker pool: %v", err)
	}

	// Register task processors
	webhookProcessor := NewWebhookProcessor(logger)
	emailProcessor := NewEmailProcessor(logger)

	if err := workerPool.RegisterTaskProcessor(types.TaskTypeWebhook, webhookProcessor); err != nil {
		log.Fatalf("Failed to register webhook processor: %v", err)
	}

	if err := workerPool.RegisterTaskProcessor(types.TaskTypeEmail, emailProcessor); err != nil {
		log.Fatalf("Failed to register email processor: %v", err)
	}

	// Start enqueueing demo tasks
	go func() {
		time.Sleep(2 * time.Second) // Wait for workers to start
		enqueueDemoTasks(queueBackend, logger)
	}()

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutdown signal received")
		cancel()
	}()

	// Start worker pool
	logger.Info("Starting worker pool...")
	if err := workerPool.Start(ctx); err != nil {
		log.Fatalf("Worker pool failed: %v", err)
	}

	// Stop worker pool gracefully
	logger.Info("Stopping worker pool...")
	if err := workerPool.Stop(context.Background()); err != nil {
		logger.Error("Failed to stop worker pool gracefully", types.Field{Key: "error", Value: err.Error()})
	}

	logger.Info("Demo completed")
}

func enqueueDemoTasks(queueBackend types.QueueBackend, logger types.Logger) {
	logger.Info("Starting to enqueue demo tasks")

	ctx := context.Background()

	// Enqueue webhook tasks
	for i := 0; i < 5; i++ {
		webhookPayload := map[string]interface{}{
			"url":    fmt.Sprintf("https://api.example.com/webhook/%d", i),
			"method": "POST",
			"body":   map[string]string{"message": fmt.Sprintf("Test webhook %d", i)},
		}

		payloadBytes, _ := json.Marshal(webhookPayload)

		task := &types.Task{
			ID:             fmt.Sprintf("webhook-%d", i),
			Type:           types.TaskTypeWebhook,
			Priority:       types.PriorityHigh,
			Queue:          "webhooks",
			Payload:        payloadBytes,
			MaxRetries:     3,
			CurrentRetries: 0,
			CreatedAt:      time.Now(),
			Timeout:        &[]time.Duration{30 * time.Second}[0],
		}

		if err := queueBackend.Enqueue(ctx, task); err != nil {
			logger.Error("Failed to enqueue webhook task", types.Field{Key: "error", Value: err.Error()})
		} else {
			logger.Info("Enqueued webhook task", types.Field{Key: "task_id", Value: task.ID})
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Enqueue email tasks
	for i := 0; i < 3; i++ {
		emailPayload := map[string]interface{}{
			"to":      fmt.Sprintf("user%d@example.com", i),
			"subject": fmt.Sprintf("Test Email %d", i),
			"body":    fmt.Sprintf("This is test email number %d", i),
		}

		payloadBytes, _ := json.Marshal(emailPayload)

		task := &types.Task{
			ID:             fmt.Sprintf("email-%d", i),
			Type:           types.TaskTypeEmail,
			Priority:       types.PriorityNormal,
			Queue:          "emails",
			Payload:        payloadBytes,
			MaxRetries:     2,
			CurrentRetries: 0,
			CreatedAt:      time.Now(),
			Timeout:        &[]time.Duration{60 * time.Second}[0],
		}

		if err := queueBackend.Enqueue(ctx, task); err != nil {
			logger.Error("Failed to enqueue email task", types.Field{Key: "error", Value: err.Error()})
		} else {
			logger.Info("Enqueued email task", types.Field{Key: "task_id", Value: task.ID})
		}

		time.Sleep(1 * time.Second)
	}

	logger.Info("Finished enqueueing demo tasks")
}
