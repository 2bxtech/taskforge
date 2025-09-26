package worker

import (
	"context"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// Event represents different events in the worker lifecycle
type Event string

const (
	// Worker lifecycle events
	EventStarted    Event = "worker_started"
	EventStopped    Event = "worker_stopped"
	EventDraining   Event = "worker_draining"
	EventHealthy    Event = "worker_healthy"
	EventUnhealthy  Event = "worker_unhealthy"
	EventRegistered Event = "worker_registered"

	// Task processing events
	TaskEventReceived  Event = "task_received"
	TaskEventStarted   Event = "task_started"
	TaskEventCompleted Event = "task_completed"
	TaskEventFailed    Event = "task_failed"
	TaskEventRetrying  Event = "task_retrying"
	TaskEventTimeout   Event = "task_timeout"

	// Queue monitoring events
	QueueEventBacklog    Event = "queue_backlog"
	QueueEventEmpty      Event = "queue_empty"
	QueueEventHighLoad   Event = "queue_high_load"
	QueueEventConnection Event = "queue_connection"

	// Circuit breaker events
	CircuitBreakerOpened   Event = "circuit_breaker_opened"
	CircuitBreakerClosed   Event = "circuit_breaker_closed"
	CircuitBreakerHalfOpen Event = "circuit_breaker_half_open"
)

// EventData contains detailed information about a worker event
type EventData struct {
	Event     Event                  `json:"event"`
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

// Observer defines the interface for observing worker events
// This enables the Observer pattern for monitoring and reacting to worker state changes
type Observer interface {
	// OnWorkerEvent is called when a worker event occurs
	OnWorkerEvent(ctx context.Context, data *EventData)

	// GetObserverID returns a unique identifier for this observer
	GetObserverID() string

	// IsActive returns whether this observer is currently active
	IsActive() bool
}

// Subject defines the interface for objects that can be observed
// Workers implement this interface to support the Observer pattern
type Subject interface {
	// RegisterObserver adds an observer to receive events
	RegisterObserver(observer Observer)

	// UnregisterObserver removes an observer
	UnregisterObserver(observerID string)

	// NotifyObservers sends an event to all registered observers
	NotifyObservers(ctx context.Context, event Event, data *EventData)
}

// EventBus implements a centralized event bus for worker events
// It manages multiple observers and provides event routing
type EventBus struct {
	observers map[string]Observer
	mutex     sync.RWMutex
	logger    types.Logger

	// Event filtering and routing
	eventFilters map[string][]Event // observerID -> events they care about

	// Buffering for high-throughput scenarios
	eventBuffer    chan *EventData
	bufferSize     int
	processingDone chan struct{}
}

// NewEventBus creates a new event bus for worker monitoring
func NewEventBus(logger types.Logger, bufferSize int) *EventBus {
	bus := &EventBus{
		observers:      make(map[string]Observer),
		eventFilters:   make(map[string][]Event),
		logger:         logger,
		eventBuffer:    make(chan *EventData, bufferSize),
		bufferSize:     bufferSize,
		processingDone: make(chan struct{}),
	}

	// Start event processing goroutine
	go bus.processEvents()

	return bus
}

// RegisterObserver adds an observer to receive all events
func (bus *EventBus) RegisterObserver(observer Observer) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	observerID := observer.GetObserverID()
	bus.observers[observerID] = observer

	bus.logger.Info("observer registered",
		types.Field{Key: "observer_id", Value: observerID},
		types.Field{Key: "total_observers", Value: len(bus.observers)})
}

// RegisterObserverWithFilter adds an observer that only receives specific events
func (bus *EventBus) RegisterObserverWithFilter(observer Observer, events []Event) {
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
func (bus *EventBus) UnregisterObserver(observerID string) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	delete(bus.observers, observerID)
	delete(bus.eventFilters, observerID)

	bus.logger.Info("observer unregistered",
		types.Field{Key: "observer_id", Value: observerID},
		types.Field{Key: "total_observers", Value: len(bus.observers)})
}

// NotifyObservers sends an event to all registered observers (async)
func (bus *EventBus) NotifyObservers(_ context.Context, event Event, data *EventData) {
	if data == nil {
		data = &EventData{}
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
func (bus *EventBus) processEvents() {
	defer close(bus.processingDone)

	for eventData := range bus.eventBuffer {
		bus.distributeEvent(context.Background(), eventData)
	}
}

// distributeEvent sends an event to all relevant observers
func (bus *EventBus) distributeEvent(ctx context.Context, eventData *EventData) {
	bus.mutex.RLock()
	observers := make(map[string]Observer)
	filters := make(map[string][]Event)

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
		go func(obs Observer, data *EventData) {
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
func (bus *EventBus) eventMatchesFilter(event Event, filter []Event) bool {
	for _, filteredEvent := range filter {
		if event == filteredEvent {
			return true
		}
	}
	return false
}

// Close shuts down the event bus gracefully
func (bus *EventBus) Close() error {
	close(bus.eventBuffer)
	<-bus.processingDone

	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	bus.observers = make(map[string]Observer)
	bus.eventFilters = make(map[string][]Event)

	bus.logger.Info("worker event bus closed")
	return nil
}

// GetObserverCount returns the number of registered observers
func (bus *EventBus) GetObserverCount() int {
	bus.mutex.RLock()
	defer bus.mutex.RUnlock()
	return len(bus.observers)
}

// GetActiveObserverCount returns the number of active observers
func (bus *EventBus) GetActiveObserverCount() int {
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
