package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// CircuitBreakerImpl implements the CircuitBreaker interface
type CircuitBreakerImpl struct {
	service string
	config  ServiceCircuitConfig
	logger  types.Logger

	// State management
	state           types.CircuitBreakerState
	lastStateChange time.Time
	nextAttemptTime time.Time
	mutex           sync.RWMutex

	// Request tracking
	totalRequests       int64
	successfulRequests  int64
	failedRequests      int64
	consecutiveFailures int

	// Rolling window for failure rate calculation
	recentRequests  []requestRecord
	windowStartTime time.Time
	windowSize      time.Duration
}

type requestRecord struct {
	timestamp time.Time
	success   bool
}

// NewCircuitBreakerImpl creates a new circuit breaker implementation
func NewCircuitBreakerImpl(service string, config ServiceCircuitConfig, logger types.Logger) *CircuitBreakerImpl {
	return &CircuitBreakerImpl{
		service:         service,
		config:          config,
		logger:          logger,
		state:           types.CircuitBreakerClosed,
		lastStateChange: time.Now(),
		windowSize:      time.Minute, // 1 minute rolling window
		windowStartTime: time.Now(),
	}
}

// Execute runs the function with circuit breaker protection
func (cb *CircuitBreakerImpl) Execute(fn func() error) error {
	// Check if we can execute
	if !cb.canExecute() {
		return fmt.Errorf("circuit breaker is open for service %s", cb.service)
	}

	// Execute the function and record result
	err := fn()
	cb.recordResult(err == nil)

	return err
}

// State returns the current circuit breaker state
func (cb *CircuitBreakerImpl) State() types.CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// Reset manually resets the circuit breaker
func (cb *CircuitBreakerImpl) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = types.CircuitBreakerClosed
	cb.consecutiveFailures = 0
	cb.lastStateChange = time.Now()
	cb.recentRequests = nil
	cb.windowStartTime = time.Now()

	cb.logger.Info("circuit breaker reset", types.Field{Key: "service", Value: cb.service})
}

// canExecute determines if a request can be executed
func (cb *CircuitBreakerImpl) canExecute() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	now := time.Now()

	switch cb.state {
	case types.CircuitBreakerClosed:
		return true

	case types.CircuitBreakerOpen:
		// Check if timeout has passed to transition to half-open
		if now.After(cb.nextAttemptTime) {
			// Transition to half-open (done in recordResult to avoid race conditions)
			return true
		}
		return false

	case types.CircuitBreakerHalfOpen:
		// Allow limited requests in half-open state
		return cb.totalRequests-cb.getRequestsInCurrentWindow() < int64(cb.config.MaxRequests)

	default:
		return false
	}
}

// recordResult records the result of a request and updates state
func (cb *CircuitBreakerImpl) recordResult(success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()

	// Update counters
	cb.totalRequests++
	if success {
		cb.successfulRequests++
		cb.consecutiveFailures = 0
	} else {
		cb.failedRequests++
		cb.consecutiveFailures++
	}

	// Add to rolling window
	cb.recentRequests = append(cb.recentRequests, requestRecord{
		timestamp: now,
		success:   success,
	})

	// Clean old records from rolling window
	cb.cleanOldRecords(now)

	// Update state based on results
	cb.updateState(now)
}

// cleanOldRecords removes records older than the window size
func (cb *CircuitBreakerImpl) cleanOldRecords(now time.Time) {
	cutoff := now.Add(-cb.windowSize)

	// Find first record within window
	start := 0
	for i, record := range cb.recentRequests {
		if record.timestamp.After(cutoff) {
			start = i
			break
		}
	}

	// Keep only recent records
	if start > 0 {
		cb.recentRequests = cb.recentRequests[start:]
	}
}

// getRequestsInCurrentWindow returns number of requests in current window
func (cb *CircuitBreakerImpl) getRequestsInCurrentWindow() int64 {
	now := time.Now()
	cutoff := now.Add(-cb.windowSize)

	count := int64(0)
	for _, record := range cb.recentRequests {
		if record.timestamp.After(cutoff) {
			count++
		}
	}

	return count
}

// calculateFailureRate calculates current failure rate in the rolling window
func (cb *CircuitBreakerImpl) calculateFailureRate() float64 {
	if len(cb.recentRequests) == 0 {
		return 0.0
	}

	now := time.Now()
	cutoff := now.Add(-cb.windowSize)

	total := 0
	failures := 0

	for _, record := range cb.recentRequests {
		if record.timestamp.After(cutoff) {
			total++
			if !record.success {
				failures++
			}
		}
	}

	if total == 0 {
		return 0.0
	}

	return float64(failures) / float64(total)
}

// updateState updates the circuit breaker state based on current conditions
func (cb *CircuitBreakerImpl) updateState(now time.Time) {
	oldState := cb.state

	switch cb.state {
	case types.CircuitBreakerClosed:
		// Check if we should open the circuit
		shouldOpen := false

		// Check consecutive failures threshold
		if cb.consecutiveFailures >= cb.config.Threshold {
			shouldOpen = true
		}

		// Check failure rate threshold
		recentRequests := cb.getRequestsInCurrentWindow()
		if recentRequests >= int64(cb.config.MinRequestCount) {
			failureRate := cb.calculateFailureRate()
			if failureRate >= cb.config.FailureThreshold {
				shouldOpen = true
			}
		}

		if shouldOpen {
			cb.state = types.CircuitBreakerOpen
			cb.nextAttemptTime = now.Add(cb.config.Timeout)
		}

	case types.CircuitBreakerOpen:
		// Check if we should transition to half-open
		if now.After(cb.nextAttemptTime) {
			cb.state = types.CircuitBreakerHalfOpen
		}

	case types.CircuitBreakerHalfOpen:
		// Check if we should close or reopen
		recentRequests := cb.getRequestsInCurrentWindow()

		if recentRequests >= int64(cb.config.MaxRequests) {
			failureRate := cb.calculateFailureRate()

			if failureRate == 0.0 {
				// No failures in half-open state, close the circuit
				cb.state = types.CircuitBreakerClosed
				cb.consecutiveFailures = 0
			} else {
				// Still failing, reopen the circuit
				cb.state = types.CircuitBreakerOpen
				cb.nextAttemptTime = now.Add(cb.config.Timeout)
			}
		}
	}

	// Log state changes
	if cb.state != oldState {
		cb.lastStateChange = now
		cb.logger.Info("circuit breaker state changed",
			types.Field{Key: "service", Value: cb.service},
			types.Field{Key: "old_state", Value: oldState},
			types.Field{Key: "new_state", Value: cb.state},
			types.Field{Key: "consecutive_failures", Value: cb.consecutiveFailures},
			types.Field{Key: "failure_rate", Value: cb.calculateFailureRate()})
	}
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreakerImpl) GetStats() CircuitBreakerStats {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return CircuitBreakerStats{
		Service:             cb.service,
		State:               cb.state,
		TotalRequests:       cb.totalRequests,
		SuccessfulRequests:  cb.successfulRequests,
		FailedRequests:      cb.failedRequests,
		ConsecutiveFailures: cb.consecutiveFailures,
		FailureRate:         cb.calculateFailureRate(),
		LastStateChange:     cb.lastStateChange,
	}
}

// RateLimiterImpl implements a token bucket rate limiter
type RateLimiterImpl struct {
	key    string
	config RateLimitSettings
	logger types.Logger

	// Token bucket state
	tokens         float64
	lastRefillTime time.Time
	mutex          sync.Mutex

	// Statistics
	totalRequests    int64
	allowedRequests  int64
	rejectedRequests int64
}

// NewRateLimiterImpl creates a new rate limiter implementation
func NewRateLimiterImpl(key string, config RateLimitSettings, logger types.Logger) *RateLimiterImpl {
	return &RateLimiterImpl{
		key:            key,
		config:         config,
		logger:         logger,
		tokens:         float64(config.BurstSize), // Start with full bucket
		lastRefillTime: time.Now(),
	}
}

// Allow checks if an operation is allowed under the rate limit
func (rl *RateLimiterImpl) Allow(ctx context.Context, key string) (bool, error) {
	return rl.AllowN(ctx, key, 1)
}

// AllowN checks if N operations are allowed
func (rl *RateLimiterImpl) AllowN(_ context.Context, _ string, n int) (bool, error) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.totalRequests++

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefillTime)

	if elapsed > 0 {
		// Add tokens based on rate
		tokensToAdd := elapsed.Seconds() * float64(rl.config.RequestsPerSecond)
		rl.tokens += tokensToAdd

		// Cap at burst size
		if rl.tokens > float64(rl.config.BurstSize) {
			rl.tokens = float64(rl.config.BurstSize)
		}

		rl.lastRefillTime = now
	}

	// Check if we have enough tokens
	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		rl.allowedRequests++
		return true, nil
	}

	// Not enough tokens
	rl.rejectedRequests++
	return false, nil
}

// Reset resets the rate limiter for a specific key
func (rl *RateLimiterImpl) Reset(_ context.Context, key string) error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.tokens = float64(rl.config.BurstSize)
	rl.lastRefillTime = time.Now()

	rl.logger.Debug("rate limiter reset", types.Field{Key: "key", Value: key})
	return nil
}

// GetStats returns rate limiter statistics
func (rl *RateLimiterImpl) GetStats() RateLimiterStats {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Calculate current rate (requests per second in last window)
	currentRate := 0.0
	if rl.totalRequests > 0 {
		// Simple approximation - could be more sophisticated
		elapsedMinutes := time.Since(rl.lastRefillTime).Minutes()
		if elapsedMinutes > 0 {
			currentRate = float64(rl.allowedRequests) / (elapsedMinutes * 60)
		}
	}

	return RateLimiterStats{
		Key:              rl.key,
		TotalRequests:    rl.totalRequests,
		AllowedRequests:  rl.allowedRequests,
		RejectedRequests: rl.rejectedRequests,
		CurrentRate:      currentRate,
		BurstCapacity:    rl.config.BurstSize,
		WindowSize:       rl.config.WindowSize,
	}
}
