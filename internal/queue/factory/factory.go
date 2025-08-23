package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/2bxtech/taskforge/internal/queue/redis"
	"github.com/2bxtech/taskforge/pkg/types"
)

// QueueBackendFactory creates queue backends using the Factory pattern
type QueueBackendFactory struct{}

// NewQueueBackendFactory creates a new queue backend factory
func NewQueueBackendFactory() *QueueBackendFactory {
	return &QueueBackendFactory{}
}

// CreateQueueBackend creates a queue backend based on the configuration
func (f *QueueBackendFactory) CreateQueueBackend(config *types.QueueConfig, logger types.Logger) (types.QueueBackend, error) {
	if config == nil {
		return nil, fmt.Errorf("queue config is required")
	}

	switch config.Backend {
	case "redis":
		return f.createRedisBackend(config, logger)
	case "postgres", "postgresql":
		return nil, fmt.Errorf("postgres backend not yet implemented")
	case "nats":
		return nil, fmt.Errorf("nats backend not yet implemented")
	case "memory":
		return nil, fmt.Errorf("memory backend not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported queue backend: %s", config.Backend)
	}
}

// createRedisBackend creates a Redis queue backend
func (f *QueueBackendFactory) createRedisBackend(config *types.QueueConfig, logger types.Logger) (types.QueueBackend, error) {
	// Convert generic queue config to Redis-specific config
	redisConfig := &redis.Config{
		Addr:         extractAddr(config.URL),
		Password:     config.Redis.Password,
		DB:           config.Redis.DB,
		PoolSize:     config.Redis.PoolSize,
		MinIdleConns: config.Redis.MinIdleConns,
		MaxRetries:   config.Redis.MaxRetries,
		DialTimeout:  config.Redis.DialTimeout,
		ReadTimeout:  config.Redis.ReadTimeout,
		WriteTimeout: config.Redis.WriteTimeout,
		PoolTimeout:  config.Redis.PoolTimeout,
	}

	// Create and return Redis queue
	return redis.NewRedisQueue(redisConfig, logger)
}

// extractAddr extracts the address from a Redis URL
func extractAddr(url string) string {
	// Simple URL parsing for Redis URLs
	// Format: redis://[password@]host:port[/db]

	if url == "" {
		return "localhost:6379"
	}

	// Remove redis:// prefix if present
	if len(url) > 8 && url[:8] == "redis://" {
		url = url[8:]
	}

	// Find @ symbol to separate password from host:port
	atIndex := -1
	for i, c := range url {
		if c == '@' {
			atIndex = i
			break
		}
	}

	// Extract host:port part
	hostPort := url
	if atIndex >= 0 {
		hostPort = url[atIndex+1:]
	}

	// Find / symbol to separate host:port from db
	slashIndex := -1
	for i, c := range hostPort {
		if c == '/' {
			slashIndex = i
			break
		}
	}

	if slashIndex >= 0 {
		hostPort = hostPort[:slashIndex]
	}

	if hostPort == "" {
		return "localhost:6379"
	}

	return hostPort
}

// QueueBackendRegistry provides a registry of available queue backends
type QueueBackendRegistry struct {
	backends map[string]QueueBackendConstructor
}

// QueueBackendConstructor is a function that creates a queue backend
type QueueBackendConstructor func(*types.QueueConfig, types.Logger) (types.QueueBackend, error)

// NewQueueBackendRegistry creates a new registry
func NewQueueBackendRegistry() *QueueBackendRegistry {
	registry := &QueueBackendRegistry{
		backends: make(map[string]QueueBackendConstructor),
	}

	// Register built-in backends
	registry.Register("redis", func(config *types.QueueConfig, logger types.Logger) (types.QueueBackend, error) {
		factory := NewQueueBackendFactory()
		return factory.createRedisBackend(config, logger)
	})

	return registry
}

// Register registers a new queue backend constructor
func (r *QueueBackendRegistry) Register(name string, constructor QueueBackendConstructor) {
	r.backends[name] = constructor
}

// Create creates a queue backend using the registered constructor
func (r *QueueBackendRegistry) Create(backendType string, config *types.QueueConfig, logger types.Logger) (types.QueueBackend, error) {
	constructor, exists := r.backends[backendType]
	if !exists {
		return nil, fmt.Errorf("unknown queue backend: %s", backendType)
	}

	return constructor(config, logger)
}

// ListAvailable returns a list of available backend types
func (r *QueueBackendRegistry) ListAvailable() []string {
	backends := make([]string, 0, len(r.backends))
	for name := range r.backends {
		backends = append(backends, name)
	}
	return backends
}

// Strategy pattern for different queue strategies
type QueueStrategy interface {
	Enqueue(backend types.QueueBackend, task *types.Task) error
	Dequeue(backend types.QueueBackend, queue string) (*types.Task, error)
}

// FIFOStrategy implements first-in-first-out queue strategy
type FIFOStrategy struct{}

// Enqueue adds a task using FIFO strategy
func (s *FIFOStrategy) Enqueue(backend types.QueueBackend, task *types.Task) error {
	return backend.Enqueue(context.Background(), task)
}

// Dequeue retrieves a task using FIFO strategy
func (s *FIFOStrategy) Dequeue(backend types.QueueBackend, queue string) (*types.Task, error) {
	return backend.Dequeue(context.Background(), queue, 5*time.Second)
}

// PriorityStrategy implements priority-based queue strategy
type PriorityStrategy struct{}

// Enqueue adds a task using priority strategy
func (s *PriorityStrategy) Enqueue(backend types.QueueBackend, task *types.Task) error {
	// The Redis backend already handles priority internally
	return backend.Enqueue(context.Background(), task)
}

// Dequeue retrieves a task using priority strategy
func (s *PriorityStrategy) Dequeue(backend types.QueueBackend, queue string) (*types.Task, error) {
	// The Redis backend already handles priority internally
	return backend.Dequeue(context.Background(), queue, 5*time.Second)
}
