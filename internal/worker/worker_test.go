package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// TestLogger implements types.Logger for testing
type TestLogger struct {
	messages []string
	mutex    sync.Mutex
}

func NewTestLogger() *TestLogger {
	return &TestLogger{messages: make([]string, 0)}
}

func (l *TestLogger) Debug(msg string, _ ...types.Field) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.messages = append(l.messages, "[DEBUG] "+msg)
}

func (l *TestLogger) Info(msg string, _ ...types.Field) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.messages = append(l.messages, "[INFO] "+msg)
}

func (l *TestLogger) Warn(msg string, _ ...types.Field) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.messages = append(l.messages, "[WARN] "+msg)
}

func (l *TestLogger) Error(msg string, _ ...types.Field) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.messages = append(l.messages, "[ERROR] "+msg)
}

func (l *TestLogger) With(_ ...types.Field) types.Logger {
	return l
}

func (l *TestLogger) GetMessages() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	messages := make([]string, len(l.messages))
	copy(messages, l.messages)
	return messages
}

// TestObserver implements Observer for testing
type TestObserver struct {
	id     string
	active bool
	events []EventData
	mutex  sync.Mutex
}

func NewTestObserver(id string) *TestObserver {
	return &TestObserver{
		id:     id,
		active: true,
		events: make([]EventData, 0),
	}
}

func (o *TestObserver) OnWorkerEvent(_ context.Context, data *EventData) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.events = append(o.events, *data)
}

func (o *TestObserver) GetObserverID() string {
	return o.id
}

func (o *TestObserver) IsActive() bool {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	return o.active
}

func (o *TestObserver) SetActive(active bool) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.active = active
}

func (o *TestObserver) GetEvents() []EventData {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	events := make([]EventData, len(o.events))
	copy(events, o.events)
	return events
}

// TestTaskProcessor implements types.TaskProcessor for testing
type TestTaskProcessor struct {
	taskType     types.TaskType
	capabilities []string
	processFn    func(ctx context.Context, task *types.Task) (*types.TaskResult, error)
}

func NewTestTaskProcessor(taskType types.TaskType) *TestTaskProcessor {
	return &TestTaskProcessor{
		taskType:     taskType,
		capabilities: []string{"test"},
		processFn: func(_ context.Context, task *types.Task) (*types.TaskResult, error) {
			return &types.TaskResult{
				TaskID: task.ID,
				Status: types.TaskStatusCompleted,
			}, nil
		},
	}
}

func (p *TestTaskProcessor) Process(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
	return p.processFn(ctx, task)
}

func (p *TestTaskProcessor) GetSupportedTypes() []types.TaskType {
	return []types.TaskType{p.taskType}
}

func (p *TestTaskProcessor) GetCapabilities() []string {
	return p.capabilities
}

func (p *TestTaskProcessor) SetProcessFn(fn func(ctx context.Context, task *types.Task) (*types.TaskResult, error)) {
	p.processFn = fn
}

// Test EventBus
func TestWorkerEventBus(t *testing.T) {
	logger := NewTestLogger()
	bus := NewEventBus(logger, 100)
	defer bus.Close()

	// Test observer registration
	observer1 := NewTestObserver("observer1")
	observer2 := NewTestObserver("observer2")

	bus.RegisterObserver(observer1)
	bus.RegisterObserver(observer2)

	if count := bus.GetObserverCount(); count != 2 {
		t.Errorf("Expected 2 observers, got %d", count)
	}

	// Test event notification
	ctx := context.Background()
	eventData := &EventData{
		Event:     EventStarted,
		WorkerID:  "test-worker",
		Timestamp: time.Now(),
	}

	bus.NotifyObservers(ctx, EventStarted, eventData)

	// Give some time for async processing
	time.Sleep(100 * time.Millisecond)

	// Check events received
	events1 := observer1.GetEvents()
	events2 := observer2.GetEvents()

	if len(events1) != 1 {
		t.Errorf("Observer1 expected 1 event, got %d", len(events1))
	}

	if len(events2) != 1 {
		t.Errorf("Observer2 expected 1 event, got %d", len(events2))
	}

	// Test observer filter
	observer3 := NewTestObserver("observer3")
	bus.RegisterObserverWithFilter(observer3, []Event{TaskEventCompleted})

	// Send different events
	bus.NotifyObservers(ctx, EventStarted, &EventData{Event: EventStarted, WorkerID: "test"})
	bus.NotifyObservers(ctx, TaskEventCompleted, &EventData{Event: TaskEventCompleted, WorkerID: "test"})

	time.Sleep(100 * time.Millisecond)

	events3 := observer3.GetEvents()
	if len(events3) != 1 || events3[0].Event != TaskEventCompleted {
		t.Errorf("Observer3 should only receive TaskEventCompleted events, got %d events", len(events3))
	}

	// Test observer unregistration
	bus.UnregisterObserver("observer1")
	if count := bus.GetObserverCount(); count != 2 {
		t.Errorf("Expected 2 observers after unregistration, got %d", count)
	}
}

// Test TaskCommandRegistry
func TestTaskCommandRegistry(t *testing.T) {
	logger := NewTestLogger()
	registry := NewTaskCommandRegistry(logger)

	// Create test processor and command
	processor := NewTestTaskProcessor(types.TaskTypeWebhook)
	command := NewBaseTaskCommand(
		types.TaskTypeWebhook,
		[]string{"test-capability"},
		ResourceRequirements{MaxMemoryMB: 128, MaxCPUPercent: 25},
		processor,
		logger,
	)

	// Test command registration
	err := registry.RegisterCommand(command)
	if err != nil {
		t.Fatalf("Failed to register command: %v", err)
	}

	// Test duplicate registration
	err = registry.RegisterCommand(command)
	if err == nil {
		t.Error("Expected error when registering duplicate command")
	}

	// Test command retrieval
	retrievedCmd, err := registry.GetCommand(types.TaskTypeWebhook)
	if err != nil {
		t.Fatalf("Failed to get command: %v", err)
	}

	if retrievedCmd != command {
		t.Error("Retrieved command is not the same as registered command")
	}

	// Test unsupported task type
	_, err = registry.GetCommand(types.TaskTypeEmail)
	if err == nil {
		t.Error("Expected error when getting unsupported task type")
	}

	// Test supported types
	supportedTypes := registry.GetSupportedTypes()
	if len(supportedTypes) != 1 || supportedTypes[0] != types.TaskTypeWebhook {
		t.Errorf("Expected 1 supported type (webhook), got %v", supportedTypes)
	}

	// Test capabilities
	capabilities := registry.GetAllCapabilities()
	if len(capabilities) != 1 || capabilities[0] != "test-capability" {
		t.Errorf("Expected 1 capability (test-capability), got %v", capabilities)
	}
}

// Test ResourcePool
func TestResourcePool(t *testing.T) {
	logger := NewTestLogger()
	config := ResourcePoolConfig{
		MaxMemoryMB:        512,
		MaxCPUPercent:      100,
		MaxConcurrentTasks: 3,
		TaskTimeout:        30 * time.Second,
	}

	pool := NewResourcePool("test-pool", config, logger)

	// Test resource acquisition
	ctx := context.Background()
	requirements := ResourceRequirements{
		MaxMemoryMB:   128,
		MaxCPUPercent: 25,
		MaxDuration:   10 * time.Second,
	}

	// Test 1: Basic acquisition
	token1, err := pool.AcquireResources(ctx, requirements)
	if err != nil {
		t.Fatalf("Failed to acquire token1: %v", err)
	}

	stats1 := pool.GetStats()
	t.Logf("After token1: activeTokens=%d, usedMemory=%d", stats1.ActiveTokens, stats1.CurrentUsage.MemoryMB)
	if stats1.ActiveTokens != 1 {
		t.Errorf("Expected 1 active token, got %d", stats1.ActiveTokens)
	}

	token2, err := pool.AcquireResources(ctx, requirements)
	if err != nil {
		t.Fatalf("Failed to acquire token2: %v", err)
	}

	stats2 := pool.GetStats()
	t.Logf("After token2: activeTokens=%d, usedMemory=%d", stats2.ActiveTokens, stats2.CurrentUsage.MemoryMB)
	if stats2.ActiveTokens != 2 {
		t.Errorf("Expected 2 active tokens, got %d", stats2.ActiveTokens)
	}

	token3, err := pool.AcquireResources(ctx, requirements)
	if err != nil {
		t.Fatalf("Failed to acquire token3: %v", err)
	}

	stats3 := pool.GetStats()
	t.Logf("After token3: activeTokens=%d, usedMemory=%d", stats3.ActiveTokens, stats3.CurrentUsage.MemoryMB)
	if stats3.ActiveTokens != 3 {
		t.Errorf("Expected 3 active tokens, got %d", stats3.ActiveTokens)
	}

	// Test 2: Resource exhaustion - fourth acquisition should fail
	_, err = pool.AcquireResources(ctx, requirements)
	if err == nil {
		t.Error("Expected error when exceeding MaxConcurrentTasks limit")
	}

	// Test 3: Release and verify count
	t.Log("Releasing token1...")
	token1.Release()

	statsAfterRelease := pool.GetStats()
	t.Logf("After token1 release: activeTokens=%d, usedMemory=%d", statsAfterRelease.ActiveTokens, statsAfterRelease.CurrentUsage.MemoryMB)
	if statsAfterRelease.ActiveTokens != 2 {
		t.Errorf("Expected 2 active tokens after release, got %d", statsAfterRelease.ActiveTokens)
	}

	// Test 4: Acquire after release
	t.Log("Acquiring token4...")
	token4, err := pool.AcquireResources(ctx, requirements)
	if err != nil {
		t.Fatalf("Failed to acquire token4 after release: %v", err)
	}

	statsAfterReacquire := pool.GetStats()
	t.Logf("After token4 acquire: activeTokens=%d, usedMemory=%d", statsAfterReacquire.ActiveTokens, statsAfterReacquire.CurrentUsage.MemoryMB)
	if statsAfterReacquire.ActiveTokens != 3 {
		t.Errorf("Expected 3 active tokens, got %d", statsAfterReacquire.ActiveTokens)
	}

	// Clean up
	token2.Release()
	token3.Release()
	token4.Release()

	finalStats := pool.GetStats()
	t.Logf("After cleanup: activeTokens=%d, usedMemory=%d", finalStats.ActiveTokens, finalStats.CurrentUsage.MemoryMB)
	if finalStats.ActiveTokens != 0 {
		t.Errorf("Expected 0 active tokens after cleanup, got %d", finalStats.ActiveTokens)
	}
}

// Test CircuitBreakerImpl
func TestCircuitBreakerImpl(t *testing.T) {
	logger := NewTestLogger()
	config := ServiceCircuitConfig{
		Threshold:        3,
		Timeout:          100 * time.Millisecond,
		MaxRequests:      2,
		ResetTimeout:     200 * time.Millisecond,
		FailureThreshold: 0.5,
		MinRequestCount:  2,
	}

	cb := NewCircuitBreakerImpl("test-service", config, logger)

	// Test initial state
	if cb.State() != types.CircuitBreakerClosed {
		t.Error("Circuit breaker should start in closed state")
	}

	// Test successful executions
	for i := 0; i < 2; i++ {
		err := cb.Execute(func() error {
			return nil // Success
		})
		if err != nil {
			t.Errorf("Execution %d failed: %v", i, err)
		}
	}

	// Test failures that should open the circuit
	for i := 0; i < 3; i++ {
		err := cb.Execute(func() error {
			return fmt.Errorf("simulated failure")
		})
		if err == nil {
			t.Errorf("Expected failure on execution %d", i)
		}
	}

	// Circuit should be open now
	if cb.State() != types.CircuitBreakerOpen {
		t.Error("Circuit breaker should be open after failures")
	}

	// Executions should fail fast
	err := cb.Execute(func() error {
		return nil
	})
	if err == nil {
		t.Error("Expected fast failure when circuit is open")
	}

	// Wait for timeout and test half-open state
	time.Sleep(150 * time.Millisecond)

	// Should allow limited requests in half-open
	err = cb.Execute(func() error {
		return nil // Success
	})
	if err != nil {
		t.Errorf("Expected success in half-open state: %v", err)
	}
}

// Test BulkheadManager
func TestBulkheadManager(t *testing.T) {
	logger := NewTestLogger()
	config := DefaultBulkheadConfig()

	// Adjust config for testing
	config.MaxTotalMemoryMB = 1024
	config.MaxTotalCPUPercent = 80
	config.MaxConcurrentTasks = 10

	manager := NewBulkheadManager(config, logger)
	defer manager.Close()

	// Test resource limiter creation
	limiter := manager.GetResourceLimiter(types.TaskTypeWebhook)
	if limiter == nil {
		t.Error("Expected resource limiter, got nil")
	}

	// Test circuit breaker creation
	cb := manager.GetCircuitBreaker("test-service")
	if cb == nil {
		t.Error("Expected circuit breaker, got nil")
	}

	if cb.State() != types.CircuitBreakerClosed {
		t.Error("Circuit breaker should start in closed state")
	}

	// Test rate limiter creation
	rl := manager.GetRateLimiter(types.TaskTypeWebhook)
	if rl == nil {
		t.Error("Expected rate limiter, got nil")
	}

	// Test resource usage tracking
	usage := manager.GetSystemResourceUsage()
	if usage.MaxTasks != config.MaxConcurrentTasks {
		t.Errorf("Expected max tasks %d, got %d", config.MaxConcurrentTasks, usage.MaxTasks)
	}

	// Test stats
	stats := manager.GetStats()
	if len(stats.ResourcePools) == 0 {
		t.Error("Expected resource pools in stats")
	}
}

// Test Health Monitor Observer
func TestHealthMonitorObserver(t *testing.T) {
	logger := NewTestLogger()
	thresholds := DefaultHealthThresholds()
	thresholds.MaxConsecutiveFailures = 2
	thresholds.MaxFailureRate = 0.5

	observer := NewHealthMonitorObserver("test-health", logger, thresholds)

	if !observer.IsActive() {
		t.Error("Observer should be active by default")
	}

	ctx := context.Background()
	workerID := "test-worker"

	// Simulate successful task completion
	observer.OnWorkerEvent(ctx, &EventData{
		Event:     TaskEventCompleted,
		WorkerID:  workerID,
		Timestamp: time.Now(),
	})

	// Worker should be healthy
	health := observer.GetWorkerHealth(workerID)
	if health == nil {
		t.Fatal("Expected worker health data")
	}

	if !health.IsHealthy {
		t.Error("Worker should be healthy after successful task")
	}

	// Simulate failures
	for i := 0; i < 3; i++ {
		observer.OnWorkerEvent(ctx, &EventData{
			Event:     TaskEventFailed,
			WorkerID:  workerID,
			Timestamp: time.Now(),
		})
	}

	// Worker should be unhealthy
	health = observer.GetWorkerHealth(workerID)
	if health.IsHealthy {
		t.Error("Worker should be unhealthy after consecutive failures")
	}

	if health.ConsecutiveFailures != 3 {
		t.Errorf("Expected 3 consecutive failures, got %d", health.ConsecutiveFailures)
	}
}

// Benchmark tests
func BenchmarkEventBusNotification(b *testing.B) {
	logger := NewTestLogger()
	bus := NewEventBus(logger, 10000)
	defer bus.Close()

	observer := NewTestObserver("bench-observer")
	bus.RegisterObserver(observer)

	ctx := context.Background()
	eventData := &EventData{
		Event:     TaskEventCompleted,
		WorkerID:  "bench-worker",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.NotifyObservers(ctx, TaskEventCompleted, eventData)
	}
}

func BenchmarkResourcePoolAcquisition(b *testing.B) {
	logger := NewTestLogger()
	config := ResourcePoolConfig{
		MaxMemoryMB:        1024,
		MaxCPUPercent:      100,
		MaxConcurrentTasks: 1000,
	}

	pool := NewResourcePool("bench-pool", config, logger)
	ctx := context.Background()
	requirements := ResourceRequirements{
		MaxMemoryMB:   1,
		MaxCPUPercent: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		token, err := pool.AcquireResources(ctx, requirements)
		if err != nil {
			b.Fatalf("Failed to acquire resources: %v", err)
		}
		token.Release()
	}
}
