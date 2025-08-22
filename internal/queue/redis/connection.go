package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ConnectionManager manages Redis connections using the Factory pattern
type ConnectionManager struct {
	client *redis.Client
	config *Config
}

// NewConnectionManager creates a new Redis connection manager
func NewConnectionManager(config *Config) (*ConnectionManager, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config is required")
	}

	// Merge with defaults and validate
	config = config.MergeWithDefaults()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid redis config: %w", err)
	}

	// Create Redis client with configuration
	client := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		MaxRetries:   config.MaxRetries,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		PoolTimeout:  config.PoolTimeout,
	})

	cm := &ConnectionManager{
		client: client,
		config: config,
	}

	return cm, nil
}

// GetClient returns the Redis client
func (cm *ConnectionManager) GetClient() *redis.Client {
	return cm.client
}

// GetConfig returns the Redis configuration
func (cm *ConnectionManager) GetConfig() *Config {
	return cm.config
}

// Ping tests the Redis connection
func (cm *ConnectionManager) Ping(ctx context.Context) error {
	_, err := cm.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

// HealthCheck performs a comprehensive health check
func (cm *ConnectionManager) HealthCheck(ctx context.Context) error {
	// Test basic connectivity
	if err := cm.Ping(ctx); err != nil {
		return err
	}

	// Test write operation
	testKey := "taskforge:healthcheck:" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := cm.client.Set(ctx, testKey, "ok", time.Second).Err(); err != nil {
		return fmt.Errorf("redis write test failed: %w", err)
	}

	// Test read operation
	if _, err := cm.client.Get(ctx, testKey).Result(); err != nil {
		return fmt.Errorf("redis read test failed: %w", err)
	}

	// Cleanup test key
	cm.client.Del(ctx, testKey)

	return nil
}

// GetStats returns Redis connection statistics
func (cm *ConnectionManager) GetStats() *redis.PoolStats {
	return cm.client.PoolStats()
}

// Close closes the Redis connection
func (cm *ConnectionManager) Close() error {
	if cm.client != nil {
		return cm.client.Close()
	}
	return nil
}

// WithRetry executes a function with retry logic
func (cm *ConnectionManager) WithRetry(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= cm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			delay := time.Duration(attempt) * 100 * time.Millisecond
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if err := operation(); err != nil {
			lastErr = err

			// Check if error is retryable
			if !isRetryableError(err) {
				return err
			}

			continue
		}

		return nil
	}

	return fmt.Errorf("operation failed after %d attempts: %w", cm.config.MaxRetries+1, lastErr)
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	// Network errors, timeouts, and temporary Redis errors are retryable
	if err == nil {
		return false
	}

	// Check for specific Redis errors that are retryable
	switch {
	case err == redis.Nil:
		return false // Not found is not retryable
	case err == context.Canceled:
		return false // Context cancelled is not retryable
	case err == context.DeadlineExceeded:
		return true // Timeout is retryable
	default:
		// Check error message for common retryable patterns
		errStr := err.Error()
		retryablePatterns := []string{
			"connection refused",
			"timeout",
			"network",
			"broken pipe",
			"connection reset",
			"temporary failure",
		}

		for _, pattern := range retryablePatterns {
			if strings.Contains(errStr, pattern) {
				return true
			}
		}
	}

	return false
}
