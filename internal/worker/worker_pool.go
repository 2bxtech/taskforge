package worker

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// WorkerPool implements the main worker pool with lifecycle management
// It orchestrates workers, observes their behavior, and manages graceful shutdown
type Pool struct {
	id              string
	config          *types.WorkerConfig
	queueBackend    types.QueueBackend
	commandRegistry *TaskCommandRegistry
	bulkheadManager *BulkheadManager
	eventBus        *EventBus
	logger          types.Logger

	// Worker management
	workers      map[string]*Instance
	workersMutex sync.RWMutex

	// Lifecycle management
	state            PoolState
	stateMutex       sync.RWMutex
	shutdownCh       chan struct{}
	shutdownComplete chan struct{}

	// Health and monitoring
	lastHeartbeat time.Time
	startTime     time.Time
	healthTicker  *time.Ticker

	// Observers
	metricsObserver *MetricsObserver
	healthObserver  *HealthMonitorObserver
}

// PoolState represents the current state of the worker pool
type PoolState string

const (
	PoolStateIdle     PoolState = "idle"     // Not started
	PoolStateStarting PoolState = "starting" // Starting up
	PoolStateRunning  PoolState = "running"  // Processing tasks
	PoolStateDraining PoolState = "draining" // Gracefully shutting down
	PoolStateStopped  PoolState = "stopped"  // Fully stopped
)

// Instance represents a single worker in the pool
type Instance struct {
	id            string
	worker        types.Worker
	queues        []string
	ctx           context.Context
	cancel        context.CancelFunc
	state         types.WorkerStatus
	lastHeartbeat time.Time
	mutex         sync.RWMutex
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(
	id string,
	config *types.WorkerConfig,
	queueBackend types.QueueBackend,
	metricsCollector types.MetricsCollector,
	logger types.Logger,
) (*Pool, error) {
	// Create command registry
	registry := NewTaskCommandRegistry(logger)

	// Create bulkhead manager with default config
	bulkheadConfig := DefaultBulkheadConfig()
	bulkheadManager := NewBulkheadManager(bulkheadConfig, logger)

	// Create event bus
	eventBus := NewEventBus(logger, 1000)

	// Create observers
	metricsObserver := NewMetricsObserver(id+"-metrics", metricsCollector, logger)
	healthObserver := NewHealthMonitorObserver(id+"-health", logger, DefaultHealthThresholds())

	// Register observers with event bus
	eventBus.RegisterObserver(metricsObserver)
	eventBus.RegisterObserver(healthObserver)

	pool := &Pool{
		id:               id,
		config:           config,
		queueBackend:     queueBackend,
		commandRegistry:  registry,
		bulkheadManager:  bulkheadManager,
		eventBus:         eventBus,
		logger:           logger,
		workers:          make(map[string]*Instance),
		state:            PoolStateIdle,
		shutdownCh:       make(chan struct{}),
		shutdownComplete: make(chan struct{}),
		metricsObserver:  metricsObserver,
		healthObserver:   healthObserver,
	}

	// Set up health monitoring callbacks
	healthObserver.SetCallbacks(
		pool.onWorkerUnhealthy,
		pool.onWorkerRecovered,
		pool.onHighFailureRate,
	)

	return pool, nil
}

// RegisterTaskProcessor registers a task processor with the worker pool
func (wp *Pool) RegisterTaskProcessor(taskType types.TaskType, processor types.TaskProcessor) error {
	// Create resource requirements based on task type
	requirements := wp.getResourceRequirementsForTaskType(taskType)

	// Create base command
	baseCommand := NewBaseTaskCommand(
		taskType,
		processor.GetCapabilities(),
		requirements,
		processor,
		wp.logger,
	)

	// Wrap with isolation features
	isolatedCommand := NewIsolatedTaskCommand(
		baseCommand,
		wp.bulkheadManager.GetResourceLimiter(taskType),
		wp.bulkheadManager.GetCircuitBreaker(string(taskType)),
		wp.bulkheadManager.GetRateLimiter(taskType),
		wp.logger,
	)

	// Register with command registry
	return wp.commandRegistry.RegisterCommand(isolatedCommand)
}

// Start starts the worker pool
func (wp *Pool) Start(ctx context.Context) error {
	wp.stateMutex.Lock()
	if wp.state != PoolStateIdle {
		wp.stateMutex.Unlock()
		return fmt.Errorf("worker pool is already started or shutting down")
	}
	wp.state = PoolStateStarting
	wp.startTime = time.Now()
	wp.stateMutex.Unlock()

	wp.logger.Info("starting worker pool",
		types.Field{Key: "pool_id", Value: wp.id},
		types.Field{Key: "concurrency", Value: wp.config.Concurrency},
		types.Field{Key: "queues", Value: wp.config.Queues})

	// Notify observers
	wp.eventBus.NotifyObservers(ctx, EventStarted, &EventData{
		Event:     EventStarted,
		WorkerID:  wp.id,
		Timestamp: time.Now(),
	})

	// Start workers
	for i := 0; i < wp.config.Concurrency; i++ {
		workerID := fmt.Sprintf("%s-worker-%d", wp.id, i)
		if err := wp.startWorker(ctx, workerID, wp.config.Queues); err != nil {
			wp.logger.Error("failed to start worker",
				types.Field{Key: "worker_id", Value: workerID},
				types.Field{Key: "error", Value: err.Error()})
			// Continue with other workers
		}
	}

	// Start health monitoring
	wp.healthTicker = time.NewTicker(wp.config.HeartbeatInterval)
	go wp.healthMonitorLoop(ctx)

	// Start task processing coordinator
	go wp.taskCoordinatorLoop(ctx)

	wp.stateMutex.Lock()
	wp.state = PoolStateRunning
	wp.stateMutex.Unlock()

	wp.logger.Info("worker pool started successfully",
		types.Field{Key: "pool_id", Value: wp.id},
		types.Field{Key: "active_workers", Value: len(wp.workers)})

	return nil
}

// Stop gracefully stops the worker pool
func (wp *Pool) Stop(ctx context.Context) error {
	wp.stateMutex.Lock()
	if wp.state == PoolStateStopped || wp.state == PoolStateDraining {
		wp.stateMutex.Unlock()
		return nil
	}

	wp.state = PoolStateDraining
	wp.stateMutex.Unlock()

	wp.logger.Info("stopping worker pool gracefully",
		types.Field{Key: "pool_id", Value: wp.id},
		types.Field{Key: "shutdown_timeout", Value: wp.config.ShutdownTimeout})

	// Notify observers of draining state
	wp.eventBus.NotifyObservers(ctx, EventDraining, &EventData{
		Event:     EventDraining,
		WorkerID:  wp.id,
		Timestamp: time.Now(),
	})

	// Signal shutdown
	close(wp.shutdownCh)

	// Create timeout context for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, wp.config.ShutdownTimeout)
	defer cancel()

	// Wait for graceful shutdown or timeout
	select {
	case <-wp.shutdownComplete:
		wp.logger.Info("worker pool stopped gracefully", types.Field{Key: "pool_id", Value: wp.id})
	case <-shutdownCtx.Done():
		wp.logger.Warn("worker pool shutdown timeout, forcing stop", types.Field{Key: "pool_id", Value: wp.id})
		wp.forceStop()
	}

	// Clean up resources
	wp.cleanup()

	wp.stateMutex.Lock()
	wp.state = PoolStateStopped
	wp.stateMutex.Unlock()

	// Final notification
	wp.eventBus.NotifyObservers(context.Background(), EventStopped, &EventData{
		Event:     EventStopped,
		WorkerID:  wp.id,
		Timestamp: time.Now(),
	})

	return nil
}

// startWorker creates and starts a new worker instance
func (wp *Pool) startWorker(ctx context.Context, workerID string, queues []string) error {
	// Create worker instance
	worker := NewWorker(workerID, wp.config, wp.queueBackend, wp.commandRegistry, wp.eventBus, wp.logger)

	// Create context for this worker
	workerCtx, cancel := context.WithCancel(ctx)

	instance := &Instance{
		id:            workerID,
		worker:        worker,
		queues:        queues,
		ctx:           workerCtx,
		cancel:        cancel,
		state:         types.WorkerStatusIdle,
		lastHeartbeat: time.Now(),
	}

	// Register worker
	wp.workersMutex.Lock()
	wp.workers[workerID] = instance
	wp.workersMutex.Unlock()

	// Start the worker
	go func() {
		defer func() {
			if r := recover(); r != nil {
				wp.logger.Error("worker panicked",
					types.Field{Key: "worker_id", Value: workerID},
					types.Field{Key: "panic", Value: r})
			}
		}()

		if err := worker.Start(workerCtx, queues); err != nil {
			wp.logger.Error("worker failed",
				types.Field{Key: "worker_id", Value: workerID},
				types.Field{Key: "error", Value: err.Error()})
		}
	}()

	// Notify observers
	wp.eventBus.NotifyObservers(ctx, EventRegistered, &EventData{
		Event:     EventRegistered,
		WorkerID:  workerID,
		Timestamp: time.Now(),
		Queue:     queues[0], // Primary queue
	})

	wp.logger.Debug("worker started",
		types.Field{Key: "worker_id", Value: workerID},
		types.Field{Key: "queues", Value: queues})

	return nil
}

// taskCoordinatorLoop coordinates task processing across workers
func (wp *Pool) taskCoordinatorLoop(ctx context.Context) {
	defer close(wp.shutdownComplete)

	for {
		select {
		case <-wp.shutdownCh:
			// Shutdown signal received
			wp.stopAllWorkers(ctx)
			return

		case <-ctx.Done():
			// Context cancelled
			wp.stopAllWorkers(ctx)
			return

		case <-time.After(1 * time.Second):
			// Periodic checks and maintenance
			wp.performMaintenance(ctx)
		}
	}
}

// healthMonitorLoop performs regular health checks
func (wp *Pool) healthMonitorLoop(ctx context.Context) {
	for {
		select {
		case <-wp.healthTicker.C:
			wp.performHealthCheck(ctx)

		case <-wp.shutdownCh:
			wp.healthTicker.Stop()
			return

		case <-ctx.Done():
			wp.healthTicker.Stop()
			return
		}
	}
}

// performHealthCheck checks the health of all workers
func (wp *Pool) performHealthCheck(ctx context.Context) {
	wp.workersMutex.RLock()
	workers := make([]*Instance, 0, len(wp.workers))
	for _, worker := range wp.workers {
		workers = append(workers, worker)
	}
	wp.workersMutex.RUnlock()

	now := time.Now()
	unhealthyWorkers := 0

	for _, worker := range workers {
		worker.mutex.RLock()
		lastHeartbeat := worker.lastHeartbeat
		state := worker.state
		worker.mutex.RUnlock()

		// Check if worker missed heartbeat
		if now.Sub(lastHeartbeat) > wp.config.HeartbeatInterval*2 {
			unhealthyWorkers++

			wp.eventBus.NotifyObservers(ctx, EventUnhealthy, &EventData{
				Event:     EventUnhealthy,
				WorkerID:  worker.id,
				Timestamp: now,
				Metadata: map[string]interface{}{
					"last_heartbeat": lastHeartbeat,
					"state":          state,
				},
			})
		} else {
			wp.eventBus.NotifyObservers(ctx, EventHealthy, &EventData{
				Event:     EventHealthy,
				WorkerID:  worker.id,
				Timestamp: now,
			})
		}
	}

	// Update pool heartbeat
	wp.lastHeartbeat = now

	wp.logger.Debug("health check completed",
		types.Field{Key: "total_workers", Value: len(workers)},
		types.Field{Key: "unhealthy_workers", Value: unhealthyWorkers})
}

// performMaintenance performs regular maintenance tasks
func (wp *Pool) performMaintenance(_ context.Context) {
	// Clean up stale worker health data
	cleaned := wp.healthObserver.Cleanup(5 * time.Minute)
	if cleaned > 0 {
		wp.logger.Debug("cleaned up stale worker health data", types.Field{Key: "count", Value: cleaned})
	}

	// Update resource usage metrics
	systemUsage := wp.bulkheadManager.GetSystemResourceUsage()
	wp.logger.Debug("system resource usage",
		types.Field{Key: "memory_available_mb", Value: systemUsage.MemoryMB},
		types.Field{Key: "cpu_available_percent", Value: systemUsage.CPUPercent},
		types.Field{Key: "active_tasks", Value: systemUsage.ActiveTasks})
}

// stopAllWorkers stops all worker instances gracefully
func (wp *Pool) stopAllWorkers(ctx context.Context) {
	wp.workersMutex.Lock()
	workers := make([]*Instance, 0, len(wp.workers))
	for _, worker := range wp.workers {
		workers = append(workers, worker)
	}
	wp.workersMutex.Unlock()

	wp.logger.Info("stopping all workers", types.Field{Key: "count", Value: len(workers)})

	// Cancel all worker contexts
	for _, worker := range workers {
		worker.cancel()
	}

	// Wait for workers to stop gracefully
	deadline := time.Now().Add(wp.config.ShutdownTimeout)
	for _, worker := range workers {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		stopCtx, cancel := context.WithTimeout(ctx, remaining)
		if err := worker.worker.Stop(stopCtx); err != nil {
			wp.logger.Warn("worker stop error",
				types.Field{Key: "worker_id", Value: worker.id},
				types.Field{Key: "error", Value: err.Error()})
		}
		cancel()
	}
}

// forceStop forcibly stops all workers
func (wp *Pool) forceStop() {
	wp.workersMutex.Lock()
	defer wp.workersMutex.Unlock()

	wp.logger.Warn("force stopping all workers")

	for _, worker := range wp.workers {
		worker.cancel()
	}

	wp.workers = make(map[string]*Instance)
}

// cleanup performs final cleanup
func (wp *Pool) cleanup() {
	// Close bulkhead manager
	if err := wp.bulkheadManager.Close(); err != nil {
		wp.logger.Error("failed to close bulkhead manager", types.Field{Key: "error", Value: err.Error()})
	}

	// Close event bus
	if err := wp.eventBus.Close(); err != nil {
		wp.logger.Error("failed to close event bus", types.Field{Key: "error", Value: err.Error()})
	}

	wp.logger.Info("worker pool cleanup completed")
}

// Health check callback implementations

// onWorkerUnhealthy handles unhealthy worker notifications
func (wp *Pool) onWorkerUnhealthy(workerID string, state *HealthState) {
	wp.logger.Warn("worker became unhealthy - considering restart",
		types.Field{Key: "worker_id", Value: workerID},
		types.Field{Key: "health_score", Value: state.HealthScore},
		types.Field{Key: "consecutive_failures", Value: state.ConsecutiveFailures})

	// Could implement auto-restart logic here
}

// onWorkerRecovered handles worker recovery notifications
func (wp *Pool) onWorkerRecovered(workerID string, state *HealthState) {
	wp.logger.Info("worker recovered",
		types.Field{Key: "worker_id", Value: workerID},
		types.Field{Key: "health_score", Value: state.HealthScore})
}

// onHighFailureRate handles high failure rate notifications
func (wp *Pool) onHighFailureRate(workerID string, failureRate float64) {
	wp.logger.Warn("worker has high failure rate",
		types.Field{Key: "worker_id", Value: workerID},
		types.Field{Key: "failure_rate", Value: failureRate})
}

// GetInfo returns information about the worker pool
func (wp *Pool) GetInfo() *PoolInfo {
	wp.stateMutex.RLock()
	state := wp.state
	wp.stateMutex.RUnlock()

	wp.workersMutex.RLock()
	workerCount := len(wp.workers)
	wp.workersMutex.RUnlock()

	hostname, _ := os.Hostname()

	return &PoolInfo{
		ID:             wp.id,
		Hostname:       hostname,
		State:          state,
		WorkerCount:    workerCount,
		Concurrency:    wp.config.Concurrency,
		Queues:         wp.config.Queues,
		SupportedTypes: wp.commandRegistry.GetSupportedTypes(),
		Capabilities:   wp.commandRegistry.GetAllCapabilities(),
		StartTime:      wp.startTime,
		LastHeartbeat:  wp.lastHeartbeat,
		ResourceUsage:  wp.bulkheadManager.GetSystemResourceUsage(),
	}
}

// WorkerPoolInfo contains information about the worker pool
type PoolInfo struct {
	ID             string             `json:"id"`
	Hostname       string             `json:"hostname"`
	State          PoolState          `json:"state"`
	WorkerCount    int                `json:"worker_count"`
	Concurrency    int                `json:"concurrency"`
	Queues         []string           `json:"queues"`
	SupportedTypes []types.TaskType   `json:"supported_types"`
	Capabilities   []string           `json:"capabilities"`
	StartTime      time.Time          `json:"start_time"`
	LastHeartbeat  time.Time          `json:"last_heartbeat"`
	ResourceUsage  AvailableResources `json:"resource_usage"`
}

// getResourceRequirementsForTaskType returns resource requirements for a task type
func (wp *Pool) getResourceRequirementsForTaskType(taskType types.TaskType) ResourceRequirements {
	// Default requirements based on task type
	switch taskType {
	case types.TaskTypeWebhook:
		return ResourceRequirements{
			MaxMemoryMB:     128,
			MaxCPUPercent:   20,
			MaxDuration:     30 * time.Second,
			RequiresNetwork: true,
			Priority:        3,
		}
	case types.TaskTypeEmail:
		return ResourceRequirements{
			MaxMemoryMB:     64,
			MaxCPUPercent:   15,
			MaxDuration:     1 * time.Minute,
			RequiresNetwork: true,
			Priority:        2,
		}
	case types.TaskTypeImageProcess:
		return ResourceRequirements{
			MaxMemoryMB:   512,
			MaxCPUPercent: 50,
			MaxDuration:   5 * time.Minute,
			RequiresGPU:   true,
			Priority:      4,
		}
	case types.TaskTypeDataProcess:
		return ResourceRequirements{
			MaxMemoryMB:   256,
			MaxCPUPercent: 40,
			MaxDuration:   10 * time.Minute,
			Priority:      3,
		}
	case types.TaskTypeBatch:
		return ResourceRequirements{
			MaxMemoryMB:   1024,
			MaxCPUPercent: 60,
			MaxDuration:   30 * time.Minute,
			Priority:      1,
		}
	default:
		return ResourceRequirements{
			MaxMemoryMB:   128,
			MaxCPUPercent: 25,
			MaxDuration:   5 * time.Minute,
			Priority:      1,
		}
	}
}
