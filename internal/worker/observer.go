package worker

import (
	"context"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// WorkerEvent represents different events in the worker lifecycle
type WorkerEvent string

const (
	// Worker lifecycle events
	WorkerEventStarted    WorkerEvent = "worker_started"
	WorkerEventStopped    WorkerEvent = "worker_stopped"
	WorkerEventDraining   WorkerEvent = "worker_draining"
	WorkerEventHealthy    WorkerEvent = "worker_healthy"
	WorkerEventUnhealthy  WorkerEvent = "worker_unhealthy"
	WorkerEventRegistered WorkerEvent = "worker_registered"

	// Task processing events
	TaskEventReceived  WorkerEvent = "task_received"
	TaskEventStarted   WorkerEvent = "task_started"
	TaskEventCompleted WorkerEvent = "task_completed"
	TaskEventFailed    WorkerEvent = "task_failed"
	TaskEventRetrying  WorkerEvent = "task_retrying"
	TaskEventTimeout   WorkerEvent = "task_timeout"

	// Queue monitoring events
	QueueEventBacklog    WorkerEvent = "queue_backlog"
	QueueEventEmpty      WorkerEvent = "queue_empty"
	QueueEventHighLoad   WorkerEvent = "queue_high_load"
	QueueEventConnection WorkerEvent = "queue_connection"

	// Circuit breaker events
	CircuitBreakerOpened   WorkerEvent = "circuit_breaker_opened"
	CircuitBreakerClosed   WorkerEvent = "circuit_breaker_closed"
	CircuitBreakerHalfOpen WorkerEvent = "circuit_breaker_half_open"
)

// WorkerEventData contains detailed information about a worker event
type WorkerEventData struct {
	Event     WorkerEvent            `json:"event"`
	WorkerID  string                 `json:"worker_id"`
	Timestamp time.Time              `json:"timestamp"`
	TaskID    string                 `json:"task_id,omitempty"`
	TaskType  types.TaskType         `json:"task_type,omitempty"`
	Queue     string                 `json:"queue,omitempty"`
	Error     error                  `json:"error,omitempty"`
	Duration  time.Duration          `json:"duration,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`

	// Health and resource metrics
	MemoryUsageMB   float64 `json:"memory_usage_mb,omitempty"`
	CPUUsagePercent float64 `json:"cpu_usage_percent,omitempty"`
	ActiveTasks     int     `json:"active_tasks,omitempty"`
	CompletedTasks  int64   `json:"completed_tasks,omitempty"`
	FailedTasks     int64   `json:"failed_tasks,omitempty"`

	// Queue metrics
	QueueDepth    int64 `json:"queue_depth,omitempty"`
	ActiveWorkers int   `json:"active_workers,omitempty"`
}

// WorkerObserver defines the interface for observing worker events
// This enables the Observer pattern for monitoring and reacting to worker state changes
type WorkerObserver interface {
	// OnWorkerEvent is called when a worker event occurs
	OnWorkerEvent(ctx context.Context, data *WorkerEventData)

	// GetObserverID returns a unique identifier for this observer
	GetObserverID() string

	// IsActive returns whether this observer is currently active
	IsActive() bool
}

// WorkerSubject defines the interface for objects that can be observed
// Workers implement this interface to support the Observer pattern
type WorkerSubject interface {
	// RegisterObserver adds an observer to receive events
	RegisterObserver(observer WorkerObserver)

	// UnregisterObserver removes an observer
	UnregisterObserver(observerID string)

	// NotifyObservers sends an event to all registered observers
	NotifyObservers(ctx context.Context, event WorkerEvent, data *WorkerEventData)
}

// WorkerEventBus implements a centralized event bus for worker events
// It manages multiple observers and provides event routing
type WorkerEventBus struct {
	observers map[string]WorkerObserver
	mutex     sync.RWMutex
	logger    types.Logger

	// Event filtering and routing
	eventFilters map[string][]WorkerEvent // observerID -> events they care about

	// Buffering for high-throughput scenarios
	eventBuffer    chan *WorkerEventData
	bufferSize     int
	processingDone chan struct{}
}

// NewWorkerEventBus creates a new event bus for worker monitoring
func NewWorkerEventBus(logger types.Logger, bufferSize int) *WorkerEventBus {
	bus := &WorkerEventBus{
		observers:      make(map[string]WorkerObserver),
		eventFilters:   make(map[string][]WorkerEvent),
		logger:         logger,
		eventBuffer:    make(chan *WorkerEventData, bufferSize),
		bufferSize:     bufferSize,
		processingDone: make(chan struct{}),
	}

	// Start event processing goroutine
	go bus.processEvents()

	return bus
}

// RegisterObserver adds an observer to receive all events
func (bus *WorkerEventBus) RegisterObserver(observer WorkerObserver) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	observerID := observer.GetObserverID()
	bus.observers[observerID] = observer

	bus.logger.Info("observer registered",
		types.Field{Key: "observer_id", Value: observerID},
		types.Field{Key: "total_observers", Value: len(bus.observers)})
}

// RegisterObserverWithFilter adds an observer that only receives specific events
func (bus *WorkerEventBus) RegisterObserverWithFilter(observer WorkerObserver, events []WorkerEvent) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	observerID := observer.GetObserverID()
	bus.observers[observerID] = observer
	bus.eventFilters[observerID] = events

	bus.logger.Info("filtered observer registered",
		types.Field{Key: "observer_id", Value: observerID},
		types.Field{Key: "filtered_events", Value: events},
		types.Field{Key: "total_observers", Value: len(bus.observers)})
}

// UnregisterObserver removes an observer
func (bus *WorkerEventBus) UnregisterObserver(observerID string) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	delete(bus.observers, observerID)
	delete(bus.eventFilters, observerID)

	bus.logger.Info("observer unregistered",
		types.Field{Key: "observer_id", Value: observerID},
		types.Field{Key: "total_observers", Value: len(bus.observers)})
}

// NotifyObservers sends an event to all registered observers (async)
func (bus *WorkerEventBus) NotifyObservers(ctx context.Context, event WorkerEvent, data *WorkerEventData) {
	if data == nil {
		data = &WorkerEventData{}
	}

	// Ensure required fields are set
	if data.Event == "" {
		data.Event = event
	}
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	select {
	case bus.eventBuffer <- data:
		// Event successfully queued
	default:
		// Buffer full, log warning and drop event
		bus.logger.Warn("event bus buffer full, dropping event",
			types.Field{Key: "event", Value: event},
			types.Field{Key: "worker_id", Value: data.WorkerID})
	}
}

// processEvents handles event distribution to observers in a separate goroutine
func (bus *WorkerEventBus) processEvents() {
	defer close(bus.processingDone)

	for eventData := range bus.eventBuffer {
		bus.distributeEvent(context.Background(), eventData)
	}
}

// distributeEvent sends an event to all relevant observers
func (bus *WorkerEventBus) distributeEvent(ctx context.Context, eventData *WorkerEventData) {
	bus.mutex.RLock()
	observers := make(map[string]WorkerObserver)
	filters := make(map[string][]WorkerEvent)

	// Copy current observers and filters
	for id, observer := range bus.observers {
		if observer.IsActive() {
			observers[id] = observer
			if filter, exists := bus.eventFilters[id]; exists {
				filters[id] = filter
			}
		}
	}
	bus.mutex.RUnlock()

	// Distribute event to relevant observers
	for id, observer := range observers {
		// Check if observer has event filter
		if filter, hasFilter := filters[id]; hasFilter {
			if !bus.eventMatchesFilter(eventData.Event, filter) {
				continue
			}
		}

		// Send event to observer in a separate goroutine to prevent blocking
		go func(obs WorkerObserver, data *WorkerEventData) {
			defer func() {
				if r := recover(); r != nil {
					bus.logger.Error("observer panic",
						types.Field{Key: "observer_id", Value: obs.GetObserverID()},
						types.Field{Key: "panic", Value: r})
				}
			}()

			// Create timeout context for observer processing
			timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			obs.OnWorkerEvent(timeoutCtx, data)
		}(observer, eventData)
	}
}

// eventMatchesFilter checks if an event matches the observer's filter
func (bus *WorkerEventBus) eventMatchesFilter(event WorkerEvent, filter []WorkerEvent) bool {
	for _, filteredEvent := range filter {
		if event == filteredEvent {
			return true
		}
	}
	return false
}

// Close shuts down the event bus gracefully
func (bus *WorkerEventBus) Close() error {
	close(bus.eventBuffer)
	<-bus.processingDone

	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	bus.observers = make(map[string]WorkerObserver)
	bus.eventFilters = make(map[string][]WorkerEvent)

	bus.logger.Info("worker event bus closed")
	return nil
}

// GetObserverCount returns the number of registered observers
func (bus *WorkerEventBus) GetObserverCount() int {
	bus.mutex.RLock()
	defer bus.mutex.RUnlock()
	return len(bus.observers)
}

// GetActiveObserverCount returns the number of active observers
func (bus *WorkerEventBus) GetActiveObserverCount() int {
	bus.mutex.RLock()
	defer bus.mutex.RUnlock()

	count := 0
	for _, observer := range bus.observers {
		if observer.IsActive() {
			count++
		}
	}
	return count
}
