//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	queueRedis "github.com/2bxtech/taskforge/internal/queue/redis"
	"github.com/2bxtech/taskforge/internal/worker"
	"github.com/2bxtech/taskforge/pkg/types"
)

type testLogger struct{}

func (testLogger) Debug(string, ...types.Field)       {}
func (testLogger) Info(string, ...types.Field)        {}
func (testLogger) Warn(string, ...types.Field)        {}
func (testLogger) Error(string, ...types.Field)       {}
func (l testLogger) With(...types.Field) types.Logger { return l }

type testMetrics struct{}

func (testMetrics) RecordTaskEnqueued(types.TaskType, types.Priority, string)                      {}
func (testMetrics) RecordTaskStarted(types.TaskType, types.Priority, string)                       {}
func (testMetrics) RecordTaskCompleted(types.TaskType, types.Priority, string, time.Duration)      {}
func (testMetrics) RecordTaskFailed(types.TaskType, types.Priority, string, time.Duration, string) {}
func (testMetrics) UpdateQueueDepth(string, int64)                                                 {}
func (testMetrics) UpdateActiveWorkers(string, int64)                                              {}
func (testMetrics) RecordWorkerRegistered(string, []string)                                        {}
func (testMetrics) RecordWorkerUnregistered(string)                                                {}
func (testMetrics) UpdateWorkerStatus(string, types.WorkerStatus)                                  {}
func (testMetrics) RecordCircuitBreakerOpen(string)                                                {}
func (testMetrics) RecordCircuitBreakerClosed(string)                                              {}
func (testMetrics) RecordCircuitBreakerHalfOpen(string)                                            {}

type failingProcessor struct {
	executions atomic.Int32
}

func (p *failingProcessor) Process(_ context.Context, task *types.Task) (*types.TaskResult, error) {
	p.executions.Add(1)
	return &types.TaskResult{TaskID: task.ID, Status: types.TaskStatusFailed}, fmt.Errorf("intentional integration failure")
}
func (*failingProcessor) GetSupportedTypes() []types.TaskType {
	return []types.TaskType{types.TaskTypeWebhook}
}
func (*failingProcessor) GetCapabilities() []string { return []string{"integration-test"} }

func TestEnqueueWorkerRetryAndDLQ(t *testing.T) {
	logger := testLogger{}
	queueConfig := queueRedis.DefaultConfig()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	queueConfig.StreamPrefix = "taskforge:integration:" + suffix + ":stream:"
	queueConfig.PrioritySetPrefix = "taskforge:integration:" + suffix + ":priority:"
	queueConfig.ScheduledSetName = "taskforge:integration:" + suffix + ":scheduled"
	queueConfig.TaskHashPrefix = "taskforge:integration:" + suffix + ":task:"
	queueConfig.ConsumerGroup = "taskforge-integration-" + suffix

	backend, err := queueRedis.NewRedisQueue(queueConfig, logger)
	if err != nil {
		t.Skipf("Redis is not available: %v", err)
	}
	defer backend.Close()

	queueName := "delivery"
	workerConfig := types.DefaultConfig().Worker
	workerConfig.Queues = []string{queueName}
	workerConfig.Concurrency = 1
	workerConfig.Timeout = 100 * time.Millisecond
	workerConfig.HeartbeatInterval = 100 * time.Millisecond
	workerConfig.ShutdownTimeout = 2 * time.Second

	pool, err := worker.NewWorkerPool("integration-pool", &workerConfig, backend, testMetrics{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	processor := &failingProcessor{}
	if err := pool.RegisterTaskProcessor(types.TaskTypeWebhook, processor); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer pool.Stop(context.Background())

	task := &types.Task{
		ID:         "delivery-" + suffix,
		Type:       types.TaskTypeWebhook,
		Priority:   types.PriorityNormal,
		Status:     types.TaskStatusPending,
		Queue:      queueName,
		CreatedAt:  time.Now(),
		MaxRetries: 1,
	}
	if err := backend.Enqueue(ctx, task); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := backend.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored != nil && stored.Status == types.TaskStatusDeadLetter {
			if got := processor.executions.Load(); got != 2 {
				t.Fatalf("processor executions = %d, want 2", got)
			}
			if stored.CurrentRetries != 1 {
				t.Fatalf("current retries = %d, want 1", stored.CurrentRetries)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("task did not reach DLQ; processor executions = %d", processor.executions.Load())
}
