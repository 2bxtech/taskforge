# TaskForge Worker Engine

This directory contains TaskForge's worker engine: a prototype implementation of fault-tolerance, resource-management, and observability patterns for task processing. See [docs/status-and-scope.md](../../docs/status-and-scope.md) for what is implemented versus scaffolding across the whole repository.

## Architecture Overview

The worker engine implements three key design patterns:

### 1. Observer Pattern (Queue State Monitoring)
- **WorkerEventBus**: Centralized event distribution system
- **WorkerObserver**: Interface for monitoring worker lifecycle events
- **Built-in Observers**:
  - **MetricsObserver**: Collects performance metrics
  - **HealthMonitorObserver**: Tracks worker health and triggers alerts

### 2. Command Pattern (Task Execution)
- **TaskCommand**: Unified interface for task processing
- **TaskCommandRegistry**: Pluggable task processor management
- **IsolatedTaskCommand**: Decorator adding resource limits and circuit breaker protection
- **BaseTaskCommand**: Common functionality for all task processors

### 3. Bulkhead Pattern (Failure Isolation)
- **BulkheadManager**: Resource pool and circuit breaker coordination
- **ResourcePool**: Isolated resource allocation per task type
- **CircuitBreaker**: Prevents cascade failures
- **RateLimiter**: Controls request rates

## Core Components

### Worker Pool
```go
type WorkerPool struct {
    // Manages multiple worker instances
    // Provides lifecycle management (start/stop)
    // Handles graceful shutdown
    // Monitors worker health
}
```

**Key Features:**
- Configurable concurrency levels
- Graceful shutdown with task completion guarantees
- Health monitoring with unhealthy/recovered event notifications
- Resource usage tracking

Health events are observed and logged, not acted on: `Pool.onWorkerUnhealthy` records the event but does not restart the worker (see `worker_pool.go`). There is no auto-restart implementation today.

### Worker Implementation
```go
type Worker struct {
    // Processes tasks using command pattern
    // Reports to observers via event bus
    // Integrates with the Redis queue backend's retry/DLQ logic
}
```

**Key Features:**
- Task type-specific processing
- Exponential backoff retry with jitter
- Dead letter queue integration
- Heartbeat monitoring
- Context-aware cancellation

### Resource Management
```go
type ResourcePool struct {
    // Enforces resource limits per task type
    // Prevents resource exhaustion
    // Tracks resource usage metrics
}
```

**Resource Types:**
- Memory allocation (MB)
- CPU usage (percentage)
- Concurrent task limits
- Execution timeouts

## Event System

### Worker Events
- `WorkerEventStarted`: Worker initialization
- `WorkerEventStopped`: Worker shutdown
- `WorkerEventHealthy`: Health check passed
- `WorkerEventUnhealthy`: Health check failed

### Task Events  
- `TaskEventReceived`: Task dequeued from Redis
- `TaskEventStarted`: Task processing began
- `TaskEventCompleted`: Task finished successfully
- `TaskEventFailed`: Task failed (will retry)
- `TaskEventRetrying`: Task scheduled for retry

### System Events
- `QueueEventBacklog`: Queue depth warning
- `CircuitBreakerOpened`: Service failing
- `CircuitBreakerClosed`: Service recovered

## Usage Examples

### Basic Worker Pool Setup
```go
// Create configuration
config := types.DefaultConfig()
config.Worker.Concurrency = 5
config.Worker.Queues = []string{"high-priority", "normal", "low"}

// Create queue backend (Redis implementation)
queueBackend, err := factory.CreateQueueBackend(&config.Queue, logger)

// Create worker pool
workerPool, err := worker.NewWorkerPool(
    "my-worker-pool",
    &config.Worker,
    queueBackend,
    metricsCollector,
    logger,
)

// Register task processors
emailProcessor := NewEmailProcessor(logger)
webhookProcessor := NewWebhookProcessor(logger)

workerPool.RegisterTaskProcessor(types.TaskTypeEmail, emailProcessor)
workerPool.RegisterTaskProcessor(types.TaskTypeWebhook, webhookProcessor)

// Start processing
ctx := context.Background()
if err := workerPool.Start(ctx); err != nil {
    log.Fatal(err)
}

// Graceful shutdown
workerPool.Stop(context.Background())
```

### Custom Task Processor
```go
type CustomProcessor struct {
    logger types.Logger
}

func (p *CustomProcessor) Process(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
    // Parse task payload
    var payload CustomPayload
    if err := task.UnmarshalPayload(&payload); err != nil {
        return nil, err
    }
    
    // Process with timeout
    select {
    case result := <-p.processAsync(payload):
        return &types.TaskResult{
            TaskID: task.ID,
            Status: types.TaskStatusCompleted,
            Result: result,
        }, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (p *CustomProcessor) GetSupportedTypes() []types.TaskType {
    return []types.TaskType{types.TaskTypeCustom}
}

func (p *CustomProcessor) GetCapabilities() []string {
    return []string{"async-processing", "timeout-handling"}
}
```

### Custom Observer
```go
type AlertingObserver struct {
    id     string
    slack  *slack.Client
}

func (o *AlertingObserver) OnWorkerEvent(ctx context.Context, data *WorkerEventData) {
    switch data.Event {
    case WorkerEventUnhealthy:
        o.sendAlert(fmt.Sprintf("Worker %s is unhealthy", data.WorkerID))
    case CircuitBreakerOpened:
        o.sendAlert(fmt.Sprintf("Circuit breaker opened for %s", data.Metadata["service"]))
    }
}

// Register with event bus
eventBus.RegisterObserverWithFilter(alertingObserver, []WorkerEvent{
    WorkerEventUnhealthy,
    CircuitBreakerOpened,
})
```

## Configuration

### Worker Configuration
```go
type WorkerConfig struct {
    ID                string        // Worker identifier
    Queues            []string      // Queues to process
    Concurrency       int           // Concurrent tasks
    Timeout           time.Duration // Task timeout
    HeartbeatInterval time.Duration // Health check interval
    ShutdownTimeout   time.Duration // Graceful shutdown timeout
    
    // Retry configuration
    MaxRetries        int           // Max retry attempts
    RetryBackoff      string        // exponential, linear, fixed
    InitialDelay      time.Duration // First retry delay
    MaxDelay          time.Duration // Maximum retry delay
    BackoffFactor     float64       // Exponential multiplier
    
    // Resource limits
    MaxMemoryMB       int           // Memory limit
    MaxCPUPercent     int           // CPU limit
    
    // Task filtering
    SupportedTypes    []TaskType    // Allowed task types
    Capabilities      []string      // Required capabilities
}
```

### Bulkhead Configuration
```go
type BulkheadConfig struct {
    // Global resource limits
    MaxTotalMemoryMB   int
    MaxTotalCPUPercent int
    MaxConcurrentTasks int
    
    // Per-task-type pools
    TaskTypePools map[TaskType]ResourcePoolConfig
    
    // Circuit breaker settings
    DefaultCircuitBreaker ServiceCircuitConfig
    ServiceCircuitBreakers map[string]ServiceCircuitConfig
    
    // Rate limiting
    TaskTypeRateLimits map[TaskType]RateLimitSettings
}
```

## Health Monitoring

### Health Thresholds
```go
type HealthThresholds struct {
    MaxConsecutiveFailures int           // 5 failures = unhealthy
    MaxFailureRate         float64       // 30% failure rate limit
    HeartbeatTimeout       time.Duration // 2 minutes without heartbeat
    MaxMemoryMB            float64       // Memory usage warning threshold
    MaxCPUPercent          float64       // CPU usage warning threshold
    MaxTaskDuration        time.Duration // Average duration warning
}
```

### Health Score Calculation
Workers receive a health score from 0.0 (unhealthy) to 1.0 (perfect health) based on:
- Consecutive failure count (-40% penalty)
- Overall failure rate (-30% penalty)  
- Resource usage (-10% penalty each for memory/CPU)
- Task processing speed (-10% penalty)
- Heartbeat timeliness (-30% penalty)

## Resource Management

### Resource Requirements
```go
type ResourceRequirements struct {
    MaxMemoryMB     int           // Memory allocation
    MaxCPUPercent   int           // CPU usage limit
    MaxDuration     time.Duration // Execution timeout
    RequiresGPU     bool          // GPU requirement
    RequiresNetwork bool          // Network access needed
    Priority        int           // Resource priority (1-5)
}
```

### Default Resource Allocations
- **Webhook**: 128MB, 20% CPU, 30s timeout
- **Email**: 64MB, 15% CPU, 1m timeout  
- **Image Processing**: 512MB, 50% CPU, 5m timeout, GPU
- **Data Processing**: 256MB, 40% CPU, 10m timeout
- **Batch Operations**: 1GB, 60% CPU, 30m timeout

## Circuit Breaker Protection

### Circuit States
- **Closed**: Normal operation, requests pass through
- **Open**: Service failing, requests fail fast
- **Half-Open**: Testing if service recovered

### Configuration
```go
type ServiceCircuitConfig struct {
    Threshold         int           // 5 failures to open
    Timeout           time.Duration // 60s stay-open duration
    MaxRequests       int           // 3 test requests in half-open
    ResetTimeout      time.Duration // 300s reset counter interval
    FailureThreshold  float64       // 50% failure rate limit
    MinRequestCount   int           // 10 requests minimum sample
}
```

## Integration with the Redis Queue Backend

The worker engine drives the `types.QueueBackend` interface implemented by `internal/queue/redis`:

### Queue Backend Integration
```go
// Uses existing QueueBackend interface
task, err := queueBackend.Dequeue(ctx, queue, timeout)
result := processor.Process(ctx, task)

// Retry logic with exponential backoff
if result.Status == types.TaskStatusFailed {
    retryAt := calculateNextRetry(task.CurrentRetries)
    queueBackend.ScheduleRetry(ctx, task.ID, retryAt)
}

// Dead letter queue for exhausted retries
if task.CurrentRetries >= task.MaxRetries {
    queueBackend.MoveToDLQ(ctx, task.ID, reason)
}
```

### Redis Streams Features
- Consumer group coordination
- Priority queue processing
- Automatic retry scheduling
- Dead letter queue management
- Task acknowledgment/NACK

## Testing

### Unit Tests
Run comprehensive unit tests:
```bash
make test
```

### Integration Tests
Requires Redis on `localhost:6379`; exercises the enqueue → worker execution → retry → DLQ path described in [docs/status-and-scope.md](../../docs/status-and-scope.md):
```bash
go test -tags=integration ./tests/integration
```

### Worker Engine Demo
Run the full demo with Redis:
```bash
# Start Redis
docker-compose up redis -d

# Run demo
make worker-demo
```

### Benchmarks
Performance testing:
```bash
make benchmark
```

## Monitoring and Metrics

`types.MetricsCollector` (`pkg/types/interfaces.go`) defines the metrics hook points the worker engine calls into: task enqueued/started/completed/failed, queue depth, active workers, worker registration, and circuit breaker state transitions.

There is no built-in metrics exporter. Nothing in this repository exposes an HTTP `/metrics` endpoint or ships a Prometheus client. `examples/worker-engine-demo/main.go` implements `MetricsCollector` by logging each call; a real deployment would implement the interface against whatever metrics system it uses (e.g. wrap `github.com/prometheus/client_golang` counters/histograms per method) and serve them itself.

## Deployment

### Recommended Settings
```go
config := &types.WorkerConfig{
    Concurrency:       runtime.NumCPU() * 2,
    HeartbeatInterval: 30 * time.Second,
    ShutdownTimeout:   60 * time.Second,
    MaxRetries:        5,
    RetryBackoff:      "exponential",
    InitialDelay:      1 * time.Second,
    MaxDelay:          5 * time.Minute,
    BackoffFactor:     2.0,
    MaxMemoryMB:       1024,
    MaxCPUPercent:     80,
}
```

### Scaling Considerations
- **Horizontal Scaling**: Multiple worker pools across instances
- **Vertical Scaling**: Increase concurrency per pool
- **Queue Partitioning**: Separate queues by priority/type
- **Resource Isolation**: Task-type-specific resource pools
- **Circuit Breakers**: Prevent cascade failures

### Suggested Alert Thresholds
These are not wired to any built-in alerting; they are starting points for an operator's own monitoring stack, driven off `HealthState.HealthScore` and `BulkheadStats`:
- Worker health score < 0.5 (unhealthy)
- Circuit breaker open for an extended period
- Queue depth growing without corresponding worker throughput
- Bulkhead rejected-request count rising

## Performance Characteristics

No throughput or latency numbers are published here. `internal/worker/worker_test.go` and `internal/queue/redis/redis_test.go` contain component-level benchmarks (`go test -bench=. -benchmem ./...`), but there is no end-to-end throughput benchmark for the worker engine, so no headline number would be honest. If you need throughput/latency figures for your workload, run `make benchmark` and the integration test against your own Redis deployment and measure it there.

## Troubleshooting

### Common Issues
1. **High Memory Usage**: Increase cleanup intervals, check for task leaks
2. **Circuit Breakers Stuck Open**: Verify downstream service health
3. **Tasks Not Processing**: Check queue connectivity and worker registration
4. **Health Scores Dropping**: Review error rates and resource usage

### Debug Logging
Enable debug logging to trace issues:
```go
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```

### Diagnostic Commands
There is no HTTP health/metrics server in this repository. Diagnostics go through code or `redis-cli` directly:
```go
// Worker pool status and per-worker info
info := workerPool.GetInfo()   // see worker_pool.go
```
```bash
# Check queue stats directly in Redis
redis-cli -c XLEN taskforge:stream:default
redis-cli -c ZCARD taskforge:scheduled
```

## Future Enhancements

### Planned Features
- **Auto-scaling**: Dynamic worker pool sizing
- **Multi-region**: Cross-region task distribution  
- **Priority Preemption**: Interrupt low-priority tasks
- **Task Dependencies**: Workflow orchestration
- **Adaptive Timeouts**: Machine learning-based timeout adjustment
- **Advanced Routing**: Smart queue assignment

### Extensibility Points
- Custom observers for monitoring integration
- Pluggable resource limiters
- Custom circuit breaker policies
- Task transformation pipelines
- Custom retry strategies

---

This worker engine provides a foundation for distributed task processing with fault tolerance, resource management, and observability features.