package worker

import (
	"context"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// MetricsObserver implements WorkerObserver to collect metrics from worker events
// It integrates with the MetricsCollector interface from types
type MetricsObserver struct {
	id        string
	collector types.MetricsCollector
	logger    types.Logger
	active    bool
	mutex     sync.RWMutex

	// Task metrics tracking
	taskStartTimes  map[string]time.Time
	workerStartTime time.Time

	// Counters for internal tracking
	eventsProcessed int64
	lastHeartbeat   time.Time
}

// NewMetricsObserver creates a new metrics observer
func NewMetricsObserver(id string, collector types.MetricsCollector, logger types.Logger) *MetricsObserver {
	return &MetricsObserver{
		id:              id,
		collector:       collector,
		logger:          logger,
		active:          true,
		taskStartTimes:  make(map[string]time.Time),
		workerStartTime: time.Now(),
		lastHeartbeat:   time.Now(),
	}
}

// GetObserverID returns the unique identifier for this observer
func (m *MetricsObserver) GetObserverID() string {
	return m.id
}

// IsActive returns whether this observer is currently active
func (m *MetricsObserver) IsActive() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.active
}

// SetActive enables or disables this observer
func (m *MetricsObserver) SetActive(active bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.active = active
}

// OnWorkerEvent processes worker events and collects relevant metrics
func (m *MetricsObserver) OnWorkerEvent(ctx context.Context, data *WorkerEventData) {
	if !m.IsActive() {
		return
	}

	m.mutex.Lock()
	m.eventsProcessed++
	m.lastHeartbeat = time.Now()
	m.mutex.Unlock()

	switch data.Event {
	case WorkerEventRegistered:
		m.collector.RecordWorkerRegistered(data.WorkerID, []string{data.Queue})

	case TaskEventReceived:
		if data.TaskType != "" && data.Queue != "" {
			// Record task enqueued (from worker perspective, it's received from queue)
			priority := m.extractPriority(data)
			m.collector.RecordTaskEnqueued(data.TaskType, priority, data.Queue)
		}

	case TaskEventStarted:
		if data.TaskID != "" && data.TaskType != "" && data.Queue != "" {
			// Track task start time for duration calculation
			m.mutex.Lock()
			m.taskStartTimes[data.TaskID] = data.Timestamp
			m.mutex.Unlock()

			priority := m.extractPriority(data)
			m.collector.RecordTaskStarted(data.TaskType, priority, data.Queue)
		}

	case TaskEventCompleted:
		if data.TaskID != "" && data.TaskType != "" && data.Queue != "" {
			duration := m.calculateTaskDuration(data.TaskID, data.Timestamp)
			priority := m.extractPriority(data)
			m.collector.RecordTaskCompleted(data.TaskType, priority, data.Queue, duration)
		}

	case TaskEventFailed:
		if data.TaskID != "" && data.TaskType != "" && data.Queue != "" {
			duration := m.calculateTaskDuration(data.TaskID, data.Timestamp)
			priority := m.extractPriority(data)
			errorType := m.extractErrorType(data.Error)
			m.collector.RecordTaskFailed(data.TaskType, priority, data.Queue, duration, errorType)
		}

	case QueueEventBacklog:
		if data.Queue != "" && data.QueueDepth > 0 {
			m.collector.UpdateQueueDepth(data.Queue, data.QueueDepth)
		}

	case WorkerEventHealthy, WorkerEventUnhealthy:
		status := types.WorkerStatusIdle
		if data.Event == WorkerEventUnhealthy {
			status = types.WorkerStatusOffline
		}
		m.collector.UpdateWorkerStatus(data.WorkerID, status)

	case CircuitBreakerOpened:
		if service := m.extractServiceName(data); service != "" {
			m.collector.RecordCircuitBreakerOpen(service)
		}

	case CircuitBreakerClosed:
		if service := m.extractServiceName(data); service != "" {
			m.collector.RecordCircuitBreakerClosed(service)
		}

	case CircuitBreakerHalfOpen:
		if service := m.extractServiceName(data); service != "" {
			m.collector.RecordCircuitBreakerHalfOpen(service)
		}
	}

	m.logger.Debug("metrics observer processed event",
		types.Field{Key: "event", Value: data.Event},
		types.Field{Key: "worker_id", Value: data.WorkerID},
		types.Field{Key: "task_id", Value: data.TaskID})
}

// calculateTaskDuration calculates how long a task took to process
func (m *MetricsObserver) calculateTaskDuration(taskID string, endTime time.Time) time.Duration {
	m.mutex.Lock()
	startTime, exists := m.taskStartTimes[taskID]
	if exists {
		delete(m.taskStartTimes, taskID)
	}
	m.mutex.Unlock()

	if !exists {
		return 0 // Unknown start time
	}

	return endTime.Sub(startTime)
}

// extractPriority extracts priority from event metadata, defaulting to normal
func (m *MetricsObserver) extractPriority(data *WorkerEventData) types.Priority {
	if data.Metadata != nil {
		if priority, ok := data.Metadata["priority"].(string); ok {
			return types.Priority(priority)
		}
	}
	return types.PriorityNormal
}

// extractErrorType extracts a categorized error type from the error
func (m *MetricsObserver) extractErrorType(err error) string {
	if err == nil {
		return "unknown"
	}

	// Categorize common error types
	errMsg := err.Error()
	switch {
	case len(errMsg) > 20 && errMsg[:20] == "context deadline":
		return "timeout"
	case len(errMsg) > 10 && errMsg[:10] == "connection":
		return "connection"
	case len(errMsg) > 11 && errMsg[:11] == "json: cannot":
		return "serialization"
	case len(errMsg) > 9 && errMsg[:9] == "validation":
		return "validation"
	default:
		return "processing"
	}
}

// extractServiceName extracts service name from event metadata
func (m *MetricsObserver) extractServiceName(data *WorkerEventData) string {
	if data.Metadata != nil {
		if service, ok := data.Metadata["service"].(string); ok {
			return service
		}
	}
	return "worker"
}

// GetStats returns statistics about this metrics observer
func (m *MetricsObserver) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"observer_id":      m.id,
		"active":           m.active,
		"events_processed": m.eventsProcessed,
		"uptime_seconds":   time.Since(m.workerStartTime).Seconds(),
		"last_heartbeat":   m.lastHeartbeat,
		"tracked_tasks":    len(m.taskStartTimes),
	}
}

// HealthMonitorObserver monitors worker health and triggers alerts/actions
type HealthMonitorObserver struct {
	id     string
	logger types.Logger
	active bool
	mutex  sync.RWMutex

	// Health tracking
	workerHealth     map[string]*WorkerHealthState
	healthThresholds HealthThresholds

	// Alert callbacks
	onUnhealthyWorker func(workerID string, state *WorkerHealthState)
	onWorkerRecovered func(workerID string, state *WorkerHealthState)
	onHighFailureRate func(workerID string, failureRate float64)
}

// WorkerHealthState tracks the health state of a worker
type WorkerHealthState struct {
	WorkerID            string
	LastSeen            time.Time
	Status              types.WorkerStatus
	ConsecutiveFailures int
	TotalTasks          int64
	FailedTasks         int64
	SuccessTasks        int64
	AvgTaskDuration     time.Duration
	MemoryUsageMB       float64
	CPUUsagePercent     float64
	ActiveTasks         int

	// Health indicators
	IsHealthy       bool
	LastHealthCheck time.Time
	HealthScore     float64 // 0.0 = unhealthy, 1.0 = perfect health
}

// HealthThresholds defines thresholds for health monitoring
type HealthThresholds struct {
	MaxConsecutiveFailures int           // Max consecutive failures before unhealthy
	MaxFailureRate         float64       // Max failure rate (0.0-1.0) before unhealthy
	HeartbeatTimeout       time.Duration // Max time without heartbeat
	MaxMemoryMB            float64       // Max memory usage before warning
	MaxCPUPercent          float64       // Max CPU usage before warning
	MaxTaskDuration        time.Duration // Max avg task duration before warning
}

// DefaultHealthThresholds returns sensible default health thresholds
func DefaultHealthThresholds() HealthThresholds {
	return HealthThresholds{
		MaxConsecutiveFailures: 5,
		MaxFailureRate:         0.3, // 30% failure rate
		HeartbeatTimeout:       2 * time.Minute,
		MaxMemoryMB:            512,
		MaxCPUPercent:          85,
		MaxTaskDuration:        10 * time.Minute,
	}
}

// NewHealthMonitorObserver creates a new health monitoring observer
func NewHealthMonitorObserver(id string, logger types.Logger, thresholds HealthThresholds) *HealthMonitorObserver {
	return &HealthMonitorObserver{
		id:               id,
		logger:           logger,
		active:           true,
		workerHealth:     make(map[string]*WorkerHealthState),
		healthThresholds: thresholds,
	}
}

// GetObserverID returns the unique identifier for this observer
func (h *HealthMonitorObserver) GetObserverID() string {
	return h.id
}

// IsActive returns whether this observer is currently active
func (h *HealthMonitorObserver) IsActive() bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return h.active
}

// SetActive enables or disables this observer
func (h *HealthMonitorObserver) SetActive(active bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.active = active
}

// SetCallbacks sets callback functions for health events
func (h *HealthMonitorObserver) SetCallbacks(
	onUnhealthy func(string, *WorkerHealthState),
	onRecovered func(string, *WorkerHealthState),
	onHighFailure func(string, float64),
) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.onUnhealthyWorker = onUnhealthy
	h.onWorkerRecovered = onRecovered
	h.onHighFailureRate = onHighFailure
}

// OnWorkerEvent processes worker events to monitor health
func (h *HealthMonitorObserver) OnWorkerEvent(ctx context.Context, data *WorkerEventData) {
	if !h.IsActive() {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Get or create worker health state
	state, exists := h.workerHealth[data.WorkerID]
	if !exists {
		state = &WorkerHealthState{
			WorkerID:    data.WorkerID,
			IsHealthy:   true,
			LastSeen:    data.Timestamp,
			Status:      types.WorkerStatusIdle,
			HealthScore: 1.0,
		}
		h.workerHealth[data.WorkerID] = state
	}

	// Update state based on event
	state.LastSeen = data.Timestamp
	h.updateWorkerHealthState(state, data)

	// Calculate health score
	oldScore := state.HealthScore
	state.HealthScore = h.calculateHealthScore(state)

	// Check for health state changes
	wasHealthy := state.IsHealthy
	state.IsHealthy = state.HealthScore > 0.5
	state.LastHealthCheck = time.Now()

	// Trigger callbacks for health state changes
	if wasHealthy && !state.IsHealthy {
		h.logger.Warn("worker became unhealthy",
			types.Field{Key: "worker_id", Value: data.WorkerID},
			types.Field{Key: "health_score", Value: state.HealthScore},
			types.Field{Key: "consecutive_failures", Value: state.ConsecutiveFailures})

		if h.onUnhealthyWorker != nil {
			go h.onUnhealthyWorker(data.WorkerID, state)
		}
	} else if !wasHealthy && state.IsHealthy {
		h.logger.Info("worker recovered",
			types.Field{Key: "worker_id", Value: data.WorkerID},
			types.Field{Key: "health_score", Value: state.HealthScore})

		if h.onWorkerRecovered != nil {
			go h.onWorkerRecovered(data.WorkerID, state)
		}
	}

	// Check for high failure rate
	if state.TotalTasks > 10 { // Only check after some tasks
		failureRate := float64(state.FailedTasks) / float64(state.TotalTasks)
		if failureRate > h.healthThresholds.MaxFailureRate && h.onHighFailureRate != nil {
			go h.onHighFailureRate(data.WorkerID, failureRate)
		}
	}

	h.logger.Debug("worker health updated",
		types.Field{Key: "worker_id", Value: data.WorkerID},
		types.Field{Key: "event", Value: data.Event},
		types.Field{Key: "old_score", Value: oldScore},
		types.Field{Key: "new_score", Value: state.HealthScore})
}

// updateWorkerHealthState updates state based on the event
func (h *HealthMonitorObserver) updateWorkerHealthState(state *WorkerHealthState, data *WorkerEventData) {
	switch data.Event {
	case TaskEventStarted:
		state.ActiveTasks++

	case TaskEventCompleted:
		state.TotalTasks++
		state.SuccessTasks++
		state.ConsecutiveFailures = 0
		if state.ActiveTasks > 0 {
			state.ActiveTasks--
		}

		// Update average task duration
		if data.Duration > 0 {
			h.updateAverageTaskDuration(state, data.Duration)
		}

	case TaskEventFailed:
		state.TotalTasks++
		state.FailedTasks++
		state.ConsecutiveFailures++
		if state.ActiveTasks > 0 {
			state.ActiveTasks--
		}

		// Update average task duration
		if data.Duration > 0 {
			h.updateAverageTaskDuration(state, data.Duration)
		}

	case WorkerEventHealthy:
		state.Status = types.WorkerStatusIdle

	case WorkerEventUnhealthy:
		state.Status = types.WorkerStatusOffline

	case WorkerEventDraining:
		state.Status = types.WorkerStatusDraining
	}

	// Update resource usage
	if data.MemoryUsageMB > 0 {
		state.MemoryUsageMB = data.MemoryUsageMB
	}
	if data.CPUUsagePercent > 0 {
		state.CPUUsagePercent = data.CPUUsagePercent
	}
	if data.ActiveTasks >= 0 {
		state.ActiveTasks = data.ActiveTasks
	}
}

// updateAverageTaskDuration updates the rolling average task duration
func (h *HealthMonitorObserver) updateAverageTaskDuration(state *WorkerHealthState, duration time.Duration) {
	// Simple exponential moving average
	alpha := 0.1 // Smoothing factor
	if state.AvgTaskDuration == 0 {
		state.AvgTaskDuration = duration
	} else {
		state.AvgTaskDuration = time.Duration(float64(state.AvgTaskDuration)*(1-alpha) + float64(duration)*alpha)
	}
}

// calculateHealthScore calculates a health score from 0.0 to 1.0
func (h *HealthMonitorObserver) calculateHealthScore(state *WorkerHealthState) float64 {
	score := 1.0

	// Penalty for consecutive failures
	if state.ConsecutiveFailures > 0 {
		failurePenalty := float64(state.ConsecutiveFailures) / float64(h.healthThresholds.MaxConsecutiveFailures)
		score -= failurePenalty * 0.4 // Up to 40% penalty
	}

	// Penalty for high failure rate
	if state.TotalTasks > 0 {
		failureRate := float64(state.FailedTasks) / float64(state.TotalTasks)
		if failureRate > h.healthThresholds.MaxFailureRate {
			ratePenalty := (failureRate - h.healthThresholds.MaxFailureRate) / (1.0 - h.healthThresholds.MaxFailureRate)
			score -= ratePenalty * 0.3 // Up to 30% penalty
		}
	}

	// Penalty for high resource usage
	if state.MemoryUsageMB > h.healthThresholds.MaxMemoryMB {
		memoryPenalty := (state.MemoryUsageMB - h.healthThresholds.MaxMemoryMB) / h.healthThresholds.MaxMemoryMB
		score -= memoryPenalty * 0.1 // Up to 10% penalty
	}

	if state.CPUUsagePercent > h.healthThresholds.MaxCPUPercent {
		cpuPenalty := (state.CPUUsagePercent - h.healthThresholds.MaxCPUPercent) / (100 - h.healthThresholds.MaxCPUPercent)
		score -= cpuPenalty * 0.1 // Up to 10% penalty
	}

	// Penalty for slow task processing
	if state.AvgTaskDuration > h.healthThresholds.MaxTaskDuration {
		durationPenalty := float64(state.AvgTaskDuration-h.healthThresholds.MaxTaskDuration) / float64(h.healthThresholds.MaxTaskDuration)
		score -= durationPenalty * 0.1 // Up to 10% penalty
	}

	// Penalty for missed heartbeats
	timeSinceLastSeen := time.Since(state.LastSeen)
	if timeSinceLastSeen > h.healthThresholds.HeartbeatTimeout {
		heartbeatPenalty := float64(timeSinceLastSeen-h.healthThresholds.HeartbeatTimeout) / float64(h.healthThresholds.HeartbeatTimeout)
		score -= heartbeatPenalty * 0.3 // Up to 30% penalty
	}

	// Ensure score is within bounds
	if score < 0.0 {
		score = 0.0
	}
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// GetWorkerHealth returns the health state for a specific worker
func (h *HealthMonitorObserver) GetWorkerHealth(workerID string) *WorkerHealthState {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	state, exists := h.workerHealth[workerID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modifications
	stateCopy := *state
	return &stateCopy
}

// GetAllWorkerHealth returns health states for all tracked workers
func (h *HealthMonitorObserver) GetAllWorkerHealth() map[string]*WorkerHealthState {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	result := make(map[string]*WorkerHealthState)
	for workerID, state := range h.workerHealth {
		stateCopy := *state
		result[workerID] = &stateCopy
	}

	return result
}

// Cleanup removes health state for workers that haven't been seen recently
func (h *HealthMonitorObserver) Cleanup(maxAge time.Duration) int {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for workerID, state := range h.workerHealth {
		if state.LastSeen.Before(cutoff) {
			delete(h.workerHealth, workerID)
			removed++

			h.logger.Info("removed stale worker health state",
				types.Field{Key: "worker_id", Value: workerID},
				types.Field{Key: "last_seen", Value: state.LastSeen})
		}
	}

	return removed
}
