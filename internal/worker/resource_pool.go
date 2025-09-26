package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// ResourcePool implements resource management for the Bulkhead pattern
// It provides isolated resource allocation with configurable limits
type ResourcePool struct {
	ID     string
	config ResourcePoolConfig
	logger types.Logger

	// Resource tracking
	activeTokens map[string]*ResourceTokenImpl
	currentUsage AvailableResources
	mutex        sync.RWMutex

	// Request tracking
	totalRequests    int64
	rejectedRequests int64
	waitTimes        []time.Duration
	tokenCounter     int64 // Add counter for unique token IDs

	// Cleanup tracking
	lastCleanup time.Time
}

// ResourceTokenImpl implements ResourceToken interface
type ResourceTokenImpl struct {
	ID           string
	requirements ResourceRequirements
	acquiredAt   time.Time
	pool         *ResourcePool
	released     bool
	mutex        sync.Mutex
}

// NewResourcePool creates a new resource pool
func NewResourcePool(id string, config ResourcePoolConfig, logger types.Logger) *ResourcePool {
	return &ResourcePool{
		ID:           id,
		config:       config,
		logger:       logger,
		activeTokens: make(map[string]*ResourceTokenImpl),
		lastCleanup:  time.Now(),
	}
}

// AcquireResources attempts to acquire resources for task execution
func (p *ResourcePool) AcquireResources(ctx context.Context, requirements ResourceRequirements) (ResourceToken, error) {
	startTime := time.Now()

	p.mutex.Lock()
	p.totalRequests++
	p.mutex.Unlock()

	// Check if resources are available
	if !p.canAllocateResources(requirements) {
		p.mutex.Lock()
		p.rejectedRequests++
		p.mutex.Unlock()

		return nil, fmt.Errorf("insufficient resources in pool %s: required(mem=%dMB, cpu=%d%%, tasks=1), available(mem=%dMB, cpu=%d%%, tasks=%d)",
			p.ID,
			requirements.MaxMemoryMB,
			requirements.MaxCPUPercent,
			p.config.MaxMemoryMB-p.currentUsage.MemoryMB,
			p.config.MaxCPUPercent-p.currentUsage.CPUPercent,
			p.config.MaxConcurrentTasks-p.currentUsage.ActiveTasks)
	}

	// Create and register token
	p.mutex.Lock()
	p.tokenCounter++
	tokenID := fmt.Sprintf("%s-%d", p.ID, p.tokenCounter)
	token := &ResourceTokenImpl{
		ID:           tokenID,
		requirements: requirements,
		acquiredAt:   time.Now(),
		pool:         p,
	}

	p.activeTokens[tokenID] = token
	p.currentUsage.MemoryMB += requirements.MaxMemoryMB
	p.currentUsage.CPUPercent += requirements.MaxCPUPercent
	p.currentUsage.ActiveTasks++

	// Track wait time
	waitTime := time.Since(startTime)
	p.waitTimes = append(p.waitTimes, waitTime)

	// Keep only recent wait times for average calculation
	if len(p.waitTimes) > 100 {
		p.waitTimes = p.waitTimes[1:]
	}
	p.mutex.Unlock()

	p.logger.Debug("resources acquired",
		types.Field{Key: "pool_id", Value: p.ID},
		types.Field{Key: "token_id", Value: tokenID},
		types.Field{Key: "memory_mb", Value: requirements.MaxMemoryMB},
		types.Field{Key: "cpu_percent", Value: requirements.MaxCPUPercent},
		types.Field{Key: "wait_time", Value: waitTime})

	return token, nil
}

// GetAvailableResources returns currently available resources
func (p *ResourcePool) GetAvailableResources() AvailableResources {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return AvailableResources{
		MemoryMB:    p.config.MaxMemoryMB - p.currentUsage.MemoryMB,
		CPUPercent:  p.config.MaxCPUPercent - p.currentUsage.CPUPercent,
		ActiveTasks: p.currentUsage.ActiveTasks,
		MaxTasks:    p.config.MaxConcurrentTasks,
	}
}

// GetCurrentUsage returns current resource usage
func (p *ResourcePool) GetCurrentUsage() AvailableResources {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.currentUsage
}

// canAllocateResources checks if the requested resources can be allocated
func (p *ResourcePool) canAllocateResources(requirements ResourceRequirements) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	// Check memory limit
	if p.currentUsage.MemoryMB+requirements.MaxMemoryMB > p.config.MaxMemoryMB {
		return false
	}

	// Check CPU limit
	if p.currentUsage.CPUPercent+requirements.MaxCPUPercent > p.config.MaxCPUPercent {
		return false
	}

	// Check concurrent task limit
	if p.currentUsage.ActiveTasks >= p.config.MaxConcurrentTasks {
		return false
	}

	return true
}

// CleanupStaleTokens removes tokens that are older than the specified cutoff
func (p *ResourcePool) CleanupStaleTokens(cutoff time.Time) int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	staleTokens := make([]*ResourceTokenImpl, 0)

	for _, token := range p.activeTokens {
		if token.acquiredAt.Before(cutoff) {
			staleTokens = append(staleTokens, token)
		}
	}

	// Force release stale tokens
	for _, token := range staleTokens {
		p.logger.Warn("force releasing stale resource token",
			types.Field{Key: "pool_id", Value: p.ID},
			types.Field{Key: "token_id", Value: token.ID},
			types.Field{Key: "acquired_at", Value: token.acquiredAt},
			types.Field{Key: "age", Value: time.Since(token.acquiredAt)})

		token.forceRelease()
	}

	p.lastCleanup = time.Now()
	return len(staleTokens)
}

// GetStats returns resource pool statistics
func (p *ResourcePool) GetStats() ResourcePoolStats {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	// Calculate average wait time
	var avgWaitTime time.Duration
	if len(p.waitTimes) > 0 {
		totalTime := time.Duration(0)
		for _, waitTime := range p.waitTimes {
			totalTime += waitTime
		}
		avgWaitTime = totalTime / time.Duration(len(p.waitTimes))
	}

	return ResourcePoolStats{
		PoolID:           p.ID,
		ActiveTokens:     len(p.activeTokens),
		TotalRequests:    p.totalRequests,
		RejectedRequests: p.rejectedRequests,
		CurrentUsage:     p.currentUsage,
		MaxUsage:         p.config,
		AverageWaitTime:  avgWaitTime,
	}
}

// Close gracefully shuts down the resource pool
func (p *ResourcePool) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Force release all active tokens
	for _, token := range p.activeTokens {
		token.forceRelease()
	}

	p.logger.Info("resource pool closed", types.Field{Key: "pool_id", Value: p.ID})
}

// ResourceTokenImpl methods

// Release returns the acquired resources to the pool
func (t *ResourceTokenImpl) Release() {
	t.mutex.Lock()
	if t.released {
		t.mutex.Unlock()
		return // Already released
	}

	// Mark as released first to prevent double release
	t.released = true
	t.mutex.Unlock()

	// Now safely update the pool
	t.pool.mutex.Lock()
	defer t.pool.mutex.Unlock()

	// Remove from active tokens
	delete(t.pool.activeTokens, t.ID)

	// Update pool usage
	t.pool.currentUsage.MemoryMB -= t.requirements.MaxMemoryMB
	t.pool.currentUsage.CPUPercent -= t.requirements.MaxCPUPercent
	t.pool.currentUsage.ActiveTasks--

	// Prevent negative values
	if t.pool.currentUsage.MemoryMB < 0 {
		t.pool.currentUsage.MemoryMB = 0
	}
	if t.pool.currentUsage.CPUPercent < 0 {
		t.pool.currentUsage.CPUPercent = 0
	}
	if t.pool.currentUsage.ActiveTasks < 0 {
		t.pool.currentUsage.ActiveTasks = 0
	}

	duration := time.Since(t.acquiredAt)
	t.pool.logger.Debug("resources released",
		types.Field{Key: "pool_id", Value: t.pool.ID},
		types.Field{Key: "token_id", Value: t.ID},
		types.Field{Key: "duration", Value: duration})
}

// GetAcquiredResources returns the resources acquired by this token
func (t *ResourceTokenImpl) GetAcquiredResources() ResourceRequirements {
	return t.requirements
}

// forceRelease is used internally for cleanup without mutex protection
func (t *ResourceTokenImpl) forceRelease() {
	if t.released {
		return
	}

	// Remove from active tokens (caller must hold pool mutex)
	delete(t.pool.activeTokens, t.ID)

	// Update pool usage
	t.pool.currentUsage.MemoryMB -= t.requirements.MaxMemoryMB
	t.pool.currentUsage.CPUPercent -= t.requirements.MaxCPUPercent
	t.pool.currentUsage.ActiveTasks--

	// Prevent negative values
	if t.pool.currentUsage.MemoryMB < 0 {
		t.pool.currentUsage.MemoryMB = 0
	}
	if t.pool.currentUsage.CPUPercent < 0 {
		t.pool.currentUsage.CPUPercent = 0
	}
	if t.pool.currentUsage.ActiveTasks < 0 {
		t.pool.currentUsage.ActiveTasks = 0
	}

	t.released = true
}
