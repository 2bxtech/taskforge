package worker

import (
	"runtime"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// BulkheadManager implements the Bulkhead pattern for failure isolation
// It manages resource pools and circuit breakers to prevent cascade failures
type BulkheadManager struct {
	resourcePools   map[string]*ResourcePool
	circuitBreakers map[string]*CircuitBreakerImpl
	rateLimiters    map[string]*RateLimiterImpl
	config          BulkheadConfig
	logger          types.Logger
	mutex           sync.RWMutex

	// Global resource tracking
	totalResourceLimits AvailableResources
	currentUsage        AvailableResources
	usageMutex          sync.RWMutex
}

// BulkheadConfig defines configuration for the bulkhead pattern
type BulkheadConfig struct {
	// Global resource limits
	MaxTotalMemoryMB   int `json:"max_total_memory_mb"`
	MaxTotalCPUPercent int `json:"max_total_cpu_percent"`
	MaxConcurrentTasks int `json:"max_concurrent_tasks"`

	// Per-task-type resource pools
	TaskTypePools map[types.TaskType]ResourcePoolConfig `json:"task_type_pools"`

	// Circuit breaker settings
	DefaultCircuitBreaker  ServiceCircuitConfig            `json:"default_circuit_breaker"`
	ServiceCircuitBreakers map[string]ServiceCircuitConfig `json:"service_circuit_breakers"`

	// Rate limiting settings
	DefaultRateLimit   RateLimitSettings                    `json:"default_rate_limit"`
	TaskTypeRateLimits map[types.TaskType]RateLimitSettings `json:"task_type_rate_limits"`

	// Health monitoring
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	ResourceCleanupAge  time.Duration `json:"resource_cleanup_age"`
}

// ResourcePoolConfig defines configuration for a resource pool
type ResourcePoolConfig struct {
	MaxMemoryMB        int           `json:"max_memory_mb"`
	MaxCPUPercent      int           `json:"max_cpu_percent"`
	MaxConcurrentTasks int           `json:"max_concurrent_tasks"`
	TaskTimeout        time.Duration `json:"task_timeout"`
	Priority           int           `json:"priority"` // Higher priority gets more resources during contention
}

// ServiceCircuitConfig defines circuit breaker settings for a service
type ServiceCircuitConfig struct {
	Threshold        int           `json:"threshold"`         // failures before opening
	Timeout          time.Duration `json:"timeout"`           // how long to stay open
	MaxRequests      int           `json:"max_requests"`      // max requests in half-open
	ResetTimeout     time.Duration `json:"reset_timeout"`     // time to reset counters
	FailureThreshold float64       `json:"failure_threshold"` // failure rate threshold (0.0-1.0)
	MinRequestCount  int           `json:"min_request_count"` // minimum requests before checking failure rate
}

// RateLimitSettings defines rate limiting configuration
type RateLimitSettings struct {
	RequestsPerSecond int           `json:"requests_per_second"`
	BurstSize         int           `json:"burst_size"`
	WindowSize        time.Duration `json:"window_size"`
}

// DefaultBulkheadConfig returns sensible default bulkhead configuration
func DefaultBulkheadConfig() BulkheadConfig {
	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)

	// Safe conversion from uint64 to int with overflow protection
	memoryMB := memStats.Sys / 1024 / 1024
	var availableMemoryMB int

	// Check for potential overflow before conversion
	if memoryMB > uint64(^uint(0)>>1) {
		// If memory exceeds int max, cap at a reasonable limit (16GB)
		availableMemoryMB = 16384
	} else {
		availableMemoryMB = int(memoryMB)
	}

	return BulkheadConfig{
		MaxTotalMemoryMB:   availableMemoryMB / 2, // Use half of available memory
		MaxTotalCPUPercent: 80,
		MaxConcurrentTasks: runtime.NumCPU() * 4,

		TaskTypePools: map[types.TaskType]ResourcePoolConfig{
			types.TaskTypeWebhook: {
				MaxMemoryMB:        128,
				MaxCPUPercent:      20,
				MaxConcurrentTasks: 10,
				TaskTimeout:        30 * time.Second,
				Priority:           3,
			},
			types.TaskTypeEmail: {
				MaxMemoryMB:        64,
				MaxCPUPercent:      15,
				MaxConcurrentTasks: 5,
				TaskTimeout:        1 * time.Minute,
				Priority:           2,
			},
			types.TaskTypeImageProcess: {
				MaxMemoryMB:        512,
				MaxCPUPercent:      50,
				MaxConcurrentTasks: 2,
				TaskTimeout:        5 * time.Minute,
				Priority:           4,
			},
			types.TaskTypeDataProcess: {
				MaxMemoryMB:        256,
				MaxCPUPercent:      40,
				MaxConcurrentTasks: 3,
				TaskTimeout:        10 * time.Minute,
				Priority:           3,
			},
			types.TaskTypeBatch: {
				MaxMemoryMB:        1024,
				MaxCPUPercent:      60,
				MaxConcurrentTasks: 1,
				TaskTimeout:        30 * time.Minute,
				Priority:           1,
			},
		},

		DefaultCircuitBreaker: ServiceCircuitConfig{
			Threshold:        5,
			Timeout:          60 * time.Second,
			MaxRequests:      3,
			ResetTimeout:     300 * time.Second,
			FailureThreshold: 0.5,
			MinRequestCount:  10,
		},

		DefaultRateLimit: RateLimitSettings{
			RequestsPerSecond: 100,
			BurstSize:         10,
			WindowSize:        1 * time.Second,
		},

		TaskTypeRateLimits: map[types.TaskType]RateLimitSettings{
			types.TaskTypeWebhook: {
				RequestsPerSecond: 50,
				BurstSize:         5,
				WindowSize:        1 * time.Second,
			},
			types.TaskTypeImageProcess: {
				RequestsPerSecond: 10,
				BurstSize:         2,
				WindowSize:        1 * time.Second,
			},
			types.TaskTypeBatch: {
				RequestsPerSecond: 5,
				BurstSize:         1,
				WindowSize:        1 * time.Second,
			},
		},

		HealthCheckInterval: 30 * time.Second,
		ResourceCleanupAge:  5 * time.Minute,
	}
}

// NewBulkheadManager creates a new bulkhead manager
func NewBulkheadManager(config BulkheadConfig, logger types.Logger) *BulkheadManager {
	manager := &BulkheadManager{
		resourcePools:   make(map[string]*ResourcePool),
		circuitBreakers: make(map[string]*CircuitBreakerImpl),
		rateLimiters:    make(map[string]*RateLimiterImpl),
		config:          config,
		logger:          logger,
		totalResourceLimits: AvailableResources{
			MemoryMB:   config.MaxTotalMemoryMB,
			CPUPercent: config.MaxTotalCPUPercent,
			MaxTasks:   config.MaxConcurrentTasks,
		},
	}

	// Initialize resource pools for each task type
	for taskType, poolConfig := range config.TaskTypePools {
		poolID := string(taskType)
		pool := NewResourcePool(poolID, poolConfig, logger)
		manager.resourcePools[poolID] = pool
	}

	// Start health monitoring
	go manager.healthMonitor()

	return manager
}

// GetResourceLimiter returns a resource limiter for the specified task type
func (b *BulkheadManager) GetResourceLimiter(taskType types.TaskType) ResourceLimiter {
	poolID := string(taskType)

	b.mutex.RLock()
	pool, exists := b.resourcePools[poolID]
	b.mutex.RUnlock()

	if !exists {
		// Create default pool for unknown task types
		b.mutex.Lock()
		pool = NewResourcePool(poolID, ResourcePoolConfig{
			MaxMemoryMB:        128,
			MaxCPUPercent:      25,
			MaxConcurrentTasks: 5,
			TaskTimeout:        5 * time.Minute,
			Priority:           1,
		}, b.logger)
		b.resourcePools[poolID] = pool
		b.mutex.Unlock()
	}

	return pool
}

// GetCircuitBreaker returns a circuit breaker for the specified service
func (b *BulkheadManager) GetCircuitBreaker(service string) types.CircuitBreaker {
	b.mutex.RLock()
	cb, exists := b.circuitBreakers[service]
	b.mutex.RUnlock()

	if !exists {
		b.mutex.Lock()
		defer b.mutex.Unlock()

		// Check again after acquiring write lock
		if cb, exists = b.circuitBreakers[service]; exists {
			return cb
		}

		// Get service-specific config or use default
		config := b.config.DefaultCircuitBreaker
		if serviceConfig, hasConfig := b.config.ServiceCircuitBreakers[service]; hasConfig {
			config = serviceConfig
		}

		cb = NewCircuitBreakerImpl(service, config, b.logger)
		b.circuitBreakers[service] = cb
	}

	return cb
}

// GetRateLimiter returns a rate limiter for the specified task type
func (b *BulkheadManager) GetRateLimiter(taskType types.TaskType) types.RateLimiter {
	key := string(taskType)

	b.mutex.RLock()
	rl, exists := b.rateLimiters[key]
	b.mutex.RUnlock()

	if !exists {
		b.mutex.Lock()
		defer b.mutex.Unlock()

		// Check again after acquiring write lock
		if rl, exists = b.rateLimiters[key]; exists {
			return rl
		}

		// Get task-type-specific config or use default
		config := b.config.DefaultRateLimit
		if taskConfig, hasConfig := b.config.TaskTypeRateLimits[taskType]; hasConfig {
			config = taskConfig
		}

		rl = NewRateLimiterImpl(key, config, b.logger)
		b.rateLimiters[key] = rl
	}

	return rl
}

// GetSystemResourceUsage returns current system resource usage
func (b *BulkheadManager) GetSystemResourceUsage() AvailableResources {
	b.usageMutex.RLock()
	defer b.usageMutex.RUnlock()

	return AvailableResources{
		MemoryMB:    b.totalResourceLimits.MemoryMB - b.currentUsage.MemoryMB,
		CPUPercent:  b.totalResourceLimits.CPUPercent - b.currentUsage.CPUPercent,
		ActiveTasks: b.currentUsage.ActiveTasks,
		MaxTasks:    b.totalResourceLimits.MaxTasks,
	}
}

// healthMonitor runs periodic health checks and cleanup
func (b *BulkheadManager) healthMonitor() {
	ticker := time.NewTicker(b.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		b.updateResourceUsage()
		b.cleanupStaleResources()
		b.checkCircuitBreakerHealth()
	}
}

// updateResourceUsage updates current resource usage from all pools
func (b *BulkheadManager) updateResourceUsage() {
	b.mutex.RLock()
	pools := make([]*ResourcePool, 0, len(b.resourcePools))
	for _, pool := range b.resourcePools {
		pools = append(pools, pool)
	}
	b.mutex.RUnlock()

	totalMemory := 0
	totalCPU := 0
	totalTasks := 0

	for _, pool := range pools {
		usage := pool.GetCurrentUsage()
		totalMemory += usage.MemoryMB
		totalCPU += usage.CPUPercent
		totalTasks += usage.ActiveTasks
	}

	b.usageMutex.Lock()
	b.currentUsage = AvailableResources{
		MemoryMB:    totalMemory,
		CPUPercent:  totalCPU,
		ActiveTasks: totalTasks,
		MaxTasks:    b.totalResourceLimits.MaxTasks,
	}
	b.usageMutex.Unlock()

	b.logger.Debug("updated system resource usage",
		types.Field{Key: "memory_mb", Value: totalMemory},
		types.Field{Key: "cpu_percent", Value: totalCPU},
		types.Field{Key: "active_tasks", Value: totalTasks})
}

// cleanupStaleResources cleans up stale resources and connections
func (b *BulkheadManager) cleanupStaleResources() {
	cutoff := time.Now().Add(-b.config.ResourceCleanupAge)

	b.mutex.RLock()
	pools := make([]*ResourcePool, 0, len(b.resourcePools))
	for _, pool := range b.resourcePools {
		pools = append(pools, pool)
	}
	b.mutex.RUnlock()

	for _, pool := range pools {
		cleaned := pool.CleanupStaleTokens(cutoff)
		if cleaned > 0 {
			b.logger.Debug("cleaned up stale resource tokens",
				types.Field{Key: "pool_id", Value: pool.ID},
				types.Field{Key: "cleaned_count", Value: cleaned})
		}
	}
}

// checkCircuitBreakerHealth monitors circuit breaker health
func (b *BulkheadManager) checkCircuitBreakerHealth() {
	b.mutex.RLock()
	breakers := make(map[string]*CircuitBreakerImpl)
	for service, cb := range b.circuitBreakers {
		breakers[service] = cb
	}
	b.mutex.RUnlock()

	for service, cb := range breakers {
		state := cb.State()
		stats := cb.GetStats()

		if state == types.CircuitBreakerOpen {
			b.logger.Warn("circuit breaker is open",
				types.Field{Key: "service", Value: service},
				types.Field{Key: "failure_rate", Value: stats.FailureRate},
				types.Field{Key: "consecutive_failures", Value: stats.ConsecutiveFailures})
		}
	}
}

// GetStats returns comprehensive bulkhead statistics
func (b *BulkheadManager) GetStats() BulkheadStats {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	poolStats := make(map[string]ResourcePoolStats)
	for id, pool := range b.resourcePools {
		poolStats[id] = pool.GetStats()
	}

	circuitBreakerStats := make(map[string]CircuitBreakerStats)
	for service, cb := range b.circuitBreakers {
		circuitBreakerStats[service] = cb.GetStats()
	}

	rateLimiterStats := make(map[string]RateLimiterStats)
	for key, rl := range b.rateLimiters {
		rateLimiterStats[key] = rl.GetStats()
	}

	return BulkheadStats{
		ResourcePools:   poolStats,
		CircuitBreakers: circuitBreakerStats,
		RateLimiters:    rateLimiterStats,
		SystemUsage:     b.GetSystemResourceUsage(),
		TotalLimits:     b.totalResourceLimits,
	}
}

// Close shuts down the bulkhead manager gracefully
func (b *BulkheadManager) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Close all resource pools
	for _, pool := range b.resourcePools {
		pool.Close()
	}

	// Reset circuit breakers
	for _, cb := range b.circuitBreakers {
		cb.Reset()
	}

	b.logger.Info("bulkhead manager closed")
	return nil
}

// BulkheadStats contains comprehensive bulkhead statistics
type BulkheadStats struct {
	ResourcePools   map[string]ResourcePoolStats   `json:"resource_pools"`
	CircuitBreakers map[string]CircuitBreakerStats `json:"circuit_breakers"`
	RateLimiters    map[string]RateLimiterStats    `json:"rate_limiters"`
	SystemUsage     AvailableResources             `json:"system_usage"`
	TotalLimits     AvailableResources             `json:"total_limits"`
}

// ResourcePoolStats contains statistics for a resource pool
type ResourcePoolStats struct {
	PoolID           string             `json:"pool_id"`
	ActiveTokens     int                `json:"active_tokens"`
	TotalRequests    int64              `json:"total_requests"`
	RejectedRequests int64              `json:"rejected_requests"`
	CurrentUsage     AvailableResources `json:"current_usage"`
	MaxUsage         ResourcePoolConfig `json:"max_usage"`
	AverageWaitTime  time.Duration      `json:"average_wait_time"`
}

// CircuitBreakerStats contains statistics for a circuit breaker
type CircuitBreakerStats struct {
	Service             string                    `json:"service"`
	State               types.CircuitBreakerState `json:"state"`
	TotalRequests       int64                     `json:"total_requests"`
	SuccessfulRequests  int64                     `json:"successful_requests"`
	FailedRequests      int64                     `json:"failed_requests"`
	ConsecutiveFailures int                       `json:"consecutive_failures"`
	FailureRate         float64                   `json:"failure_rate"`
	LastStateChange     time.Time                 `json:"last_state_change"`
}

// RateLimiterStats contains statistics for a rate limiter
type RateLimiterStats struct {
	Key              string        `json:"key"`
	TotalRequests    int64         `json:"total_requests"`
	AllowedRequests  int64         `json:"allowed_requests"`
	RejectedRequests int64         `json:"rejected_requests"`
	CurrentRate      float64       `json:"current_rate"`
	BurstCapacity    int           `json:"burst_capacity"`
	WindowSize       time.Duration `json:"window_size"`
}
