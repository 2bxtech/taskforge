package redis

import (
	"fmt"
	"time"
)

// Config contains Redis-specific queue configuration
type Config struct {
	// Connection settings
	Addr     string `json:"addr" yaml:"addr"`         // Redis server address
	Password string `json:"password" yaml:"password"` // Redis password
	DB       int    `json:"db" yaml:"db"`             // Redis database number

	// Connection pool settings
	PoolSize     int `json:"pool_size" yaml:"pool_size"`           // Maximum number of connections
	MinIdleConns int `json:"min_idle_conns" yaml:"min_idle_conns"` // Minimum idle connections
	MaxRetries   int `json:"max_retries" yaml:"max_retries"`       // Maximum retry attempts

	// Timeout settings
	DialTimeout  time.Duration `json:"dial_timeout" yaml:"dial_timeout"`   // Connection timeout
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`   // Read operation timeout
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"` // Write operation timeout
	PoolTimeout  time.Duration `json:"pool_timeout" yaml:"pool_timeout"`   // Pool checkout timeout

	// Stream settings
	StreamPrefix     string        `json:"stream_prefix" yaml:"stream_prefix"`             // Prefix for stream names
	ConsumerGroup    string        `json:"consumer_group" yaml:"consumer_group"`           // Consumer group name
	ConsumerPrefix   string        `json:"consumer_prefix" yaml:"consumer_prefix"`         // Prefix for consumer names
	BlockTime        time.Duration `json:"block_time" yaml:"block_time"`                   // XREADGROUP block time
	ClaimMinIdleTime time.Duration `json:"claim_min_idle_time" yaml:"claim_min_idle_time"` // Min idle time before claiming

	// Priority queue settings
	PrioritySetPrefix string `json:"priority_set_prefix" yaml:"priority_set_prefix"` // Prefix for priority sorted sets

	// Dead letter queue settings
	DLQSuffix     string `json:"dlq_suffix" yaml:"dlq_suffix"`           // Suffix for DLQ stream names
	DLQMaxEntries int64  `json:"dlq_max_entries" yaml:"dlq_max_entries"` // Maximum entries in DLQ

	// Task storage settings
	TaskHashPrefix string        `json:"task_hash_prefix" yaml:"task_hash_prefix"` // Prefix for task hash keys
	TaskTTL        time.Duration `json:"task_ttl" yaml:"task_ttl"`                 // Task expiration time

	// Cleanup settings
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`   // Cleanup job interval
	MaxStreamLength int64         `json:"max_stream_length" yaml:"max_stream_length"` // Maximum stream entries

	// Monitoring settings
	EnableMetrics   bool          `json:"enable_metrics" yaml:"enable_metrics"`     // Enable metrics collection
	MetricsInterval time.Duration `json:"metrics_interval" yaml:"metrics_interval"` // Metrics collection interval
}

// Validate validates the Redis configuration
func (c *Config) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("redis address is required")
	}

	if c.PoolSize <= 0 {
		return fmt.Errorf("pool size must be greater than 0")
	}

	if c.MinIdleConns < 0 {
		return fmt.Errorf("min idle connections cannot be negative")
	}

	if c.MinIdleConns > c.PoolSize {
		return fmt.Errorf("min idle connections cannot exceed pool size")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}

	if c.DialTimeout <= 0 {
		return fmt.Errorf("dial timeout must be greater than 0")
	}

	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read timeout must be greater than 0")
	}

	if c.WriteTimeout <= 0 {
		return fmt.Errorf("write timeout must be greater than 0")
	}

	if c.StreamPrefix == "" {
		return fmt.Errorf("stream prefix is required")
	}

	if c.ConsumerGroup == "" {
		return fmt.Errorf("consumer group is required")
	}

	if c.BlockTime < 0 {
		return fmt.Errorf("block time cannot be negative")
	}

	return nil
}

// DefaultConfig returns a Redis configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		// Connection settings
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,

		// Connection pool settings
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,

		// Timeout settings
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,

		// Stream settings
		StreamPrefix:     "taskforge:stream:",
		ConsumerGroup:    "taskforge-workers",
		ConsumerPrefix:   "worker:",
		BlockTime:        5 * time.Second,
		ClaimMinIdleTime: 30 * time.Second,

		// Priority queue settings
		PrioritySetPrefix: "taskforge:priority:",

		// Dead letter queue settings
		DLQSuffix:     ":dlq",
		DLQMaxEntries: 10000,

		// Task storage settings
		TaskHashPrefix: "taskforge:task:",
		TaskTTL:        24 * time.Hour,

		// Cleanup settings
		CleanupInterval: 5 * time.Minute,
		MaxStreamLength: 100000,

		// Monitoring settings
		EnableMetrics:   true,
		MetricsInterval: 30 * time.Second,
	}
}

// MergeWithDefaults merges the provided config with defaults
func (c *Config) MergeWithDefaults() *Config {
	defaults := DefaultConfig()

	if c.Addr == "" {
		c.Addr = defaults.Addr
	}
	if c.PoolSize == 0 {
		c.PoolSize = defaults.PoolSize
	}
	if c.MinIdleConns == 0 {
		c.MinIdleConns = defaults.MinIdleConns
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaults.MaxRetries
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = defaults.DialTimeout
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = defaults.ReadTimeout
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = defaults.WriteTimeout
	}
	if c.PoolTimeout == 0 {
		c.PoolTimeout = defaults.PoolTimeout
	}
	if c.StreamPrefix == "" {
		c.StreamPrefix = defaults.StreamPrefix
	}
	if c.ConsumerGroup == "" {
		c.ConsumerGroup = defaults.ConsumerGroup
	}
	if c.ConsumerPrefix == "" {
		c.ConsumerPrefix = defaults.ConsumerPrefix
	}
	if c.BlockTime == 0 {
		c.BlockTime = defaults.BlockTime
	}
	if c.ClaimMinIdleTime == 0 {
		c.ClaimMinIdleTime = defaults.ClaimMinIdleTime
	}
	if c.PrioritySetPrefix == "" {
		c.PrioritySetPrefix = defaults.PrioritySetPrefix
	}
	if c.DLQSuffix == "" {
		c.DLQSuffix = defaults.DLQSuffix
	}
	if c.DLQMaxEntries == 0 {
		c.DLQMaxEntries = defaults.DLQMaxEntries
	}
	if c.TaskHashPrefix == "" {
		c.TaskHashPrefix = defaults.TaskHashPrefix
	}
	if c.TaskTTL == 0 {
		c.TaskTTL = defaults.TaskTTL
	}
	if c.CleanupInterval == 0 {
		c.CleanupInterval = defaults.CleanupInterval
	}
	if c.MaxStreamLength == 0 {
		c.MaxStreamLength = defaults.MaxStreamLength
	}
	if c.MetricsInterval == 0 {
		c.MetricsInterval = defaults.MetricsInterval
	}

	return c
}

// GetStreamName returns the Redis stream name for a queue
func (c *Config) GetStreamName(queue string) string {
	return c.StreamPrefix + queue
}

// GetPrioritySetName returns the Redis sorted set name for priority queuing
func (c *Config) GetPrioritySetName(queue string) string {
	return c.PrioritySetPrefix + queue
}

// GetDLQStreamName returns the Redis stream name for dead letter queue
func (c *Config) GetDLQStreamName(queue string) string {
	return c.StreamPrefix + queue + c.DLQSuffix
}

// GetTaskHashKey returns the Redis hash key for task storage
func (c *Config) GetTaskHashKey(taskID string) string {
	return c.TaskHashPrefix + taskID
}

// GetConsumerName returns a consumer name with the configured prefix
func (c *Config) GetConsumerName(workerID string) string {
	return c.ConsumerPrefix + workerID
}
