package types

import (
	"time"
)

// Config represents the main configuration for TaskForge
type Config struct {
	// Application settings
	App AppConfig `json:"app" yaml:"app"`

	// Queue backend configuration
	Queue QueueConfig `json:"queue" yaml:"queue"`

	// Worker configuration
	Worker WorkerConfig `json:"worker" yaml:"worker"`

	// Scheduler configuration
	Scheduler SchedulerConfig `json:"scheduler" yaml:"scheduler"`

	// API server configuration
	API APIConfig `json:"api" yaml:"api"`

	// Monitoring and observability
	Metrics MetricsConfig `json:"metrics" yaml:"metrics"`
	Logging LoggingConfig `json:"logging" yaml:"logging"`

	// Circuit breaker configuration
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker" yaml:"circuit_breaker"`

	// Rate limiting configuration
	RateLimit RateLimitConfig `json:"rate_limit" yaml:"rate_limit"`

	// Security configuration
	Security SecurityConfig `json:"security" yaml:"security"`
}

// AppConfig contains general application settings
type AppConfig struct {
	Name        string `json:"name" yaml:"name"`
	Version     string `json:"version" yaml:"version"`
	Environment string `json:"environment" yaml:"environment"` // dev, staging, prod
	Debug       bool   `json:"debug" yaml:"debug"`
}

// QueueConfig contains queue backend configuration
type QueueConfig struct {
	Backend    string        `json:"backend" yaml:"backend"`         // redis, postgres, nats
	URL        string        `json:"url" yaml:"url"`                 // Connection URL
	MaxRetries int           `json:"max_retries" yaml:"max_retries"` // Connection retries
	Timeout    time.Duration `json:"timeout" yaml:"timeout"`         // Operation timeout

	// Redis-specific settings
	Redis RedisConfig `json:"redis" yaml:"redis"`

	// PostgreSQL-specific settings
	Postgres PostgresConfig `json:"postgres" yaml:"postgres"`

	// Default queue settings
	DefaultQueue string `json:"default_queue" yaml:"default_queue"`
	MaxQueueSize int    `json:"max_queue_size" yaml:"max_queue_size"`

	// Task retention
	CompletedTaskTTL time.Duration `json:"completed_task_ttl" yaml:"completed_task_ttl"`
	FailedTaskTTL    time.Duration `json:"failed_task_ttl" yaml:"failed_task_ttl"`
}

// RedisConfig contains Redis-specific configuration
type RedisConfig struct {
	DB           int           `json:"db" yaml:"db"`
	Password     string        `json:"password" yaml:"password"`
	MaxRetries   int           `json:"max_retries" yaml:"max_retries"`
	PoolSize     int           `json:"pool_size" yaml:"pool_size"`
	MinIdleConns int           `json:"min_idle_conns" yaml:"min_idle_conns"`
	DialTimeout  time.Duration `json:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`
	PoolTimeout  time.Duration `json:"pool_timeout" yaml:"pool_timeout"`
}

// PostgresConfig contains PostgreSQL-specific configuration
type PostgresConfig struct {
	Host           string        `json:"host" yaml:"host"`
	Port           int           `json:"port" yaml:"port"`
	Database       string        `json:"database" yaml:"database"`
	Username       string        `json:"username" yaml:"username"`
	Password       string        `json:"password" yaml:"password"`
	SSLMode        string        `json:"ssl_mode" yaml:"ssl_mode"`
	MaxConnections int           `json:"max_connections" yaml:"max_connections"`
	MaxIdleTime    time.Duration `json:"max_idle_time" yaml:"max_idle_time"`
	MaxLifetime    time.Duration `json:"max_lifetime" yaml:"max_lifetime"`
	ConnectTimeout time.Duration `json:"connect_timeout" yaml:"connect_timeout"`
}

// WorkerConfig contains worker configuration
type WorkerConfig struct {
	ID                string        `json:"id" yaml:"id"`                   // Worker identifier
	Queues            []string      `json:"queues" yaml:"queues"`           // Queues to process
	Concurrency       int           `json:"concurrency" yaml:"concurrency"` // Concurrent tasks
	Timeout           time.Duration `json:"timeout" yaml:"timeout"`         // Task timeout
	HeartbeatInterval time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	ShutdownTimeout   time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`

	// Retry configuration
	MaxRetries    int           `json:"max_retries" yaml:"max_retries"`
	RetryBackoff  string        `json:"retry_backoff" yaml:"retry_backoff"` // exponential, linear, fixed
	InitialDelay  time.Duration `json:"initial_delay" yaml:"initial_delay"`
	MaxDelay      time.Duration `json:"max_delay" yaml:"max_delay"`
	BackoffFactor float64       `json:"backoff_factor" yaml:"backoff_factor"`

	// Resource limits
	MaxMemoryMB   int `json:"max_memory_mb" yaml:"max_memory_mb"`
	MaxCPUPercent int `json:"max_cpu_percent" yaml:"max_cpu_percent"`

	// Task type filters
	SupportedTypes []TaskType `json:"supported_types" yaml:"supported_types"`
	Capabilities   []string   `json:"capabilities" yaml:"capabilities"`
}

// SchedulerConfig contains scheduler configuration
type SchedulerConfig struct {
	Enabled           bool          `json:"enabled" yaml:"enabled"`
	CheckInterval     time.Duration `json:"check_interval" yaml:"check_interval"`
	MaxScheduledTasks int           `json:"max_scheduled_tasks" yaml:"max_scheduled_tasks"`
	Timezone          string        `json:"timezone" yaml:"timezone"`

	// Cleanup settings
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	CleanupAge      time.Duration `json:"cleanup_age" yaml:"cleanup_age"`
}

// APIConfig contains API server configuration
type APIConfig struct {
	Enabled bool      `json:"enabled" yaml:"enabled"`
	Host    string    `json:"host" yaml:"host"`
	Port    int       `json:"port" yaml:"port"`
	TLS     TLSConfig `json:"tls" yaml:"tls"`

	// Timeouts
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout" yaml:"idle_timeout"`

	// Middleware
	EnableCORS    bool `json:"enable_cors" yaml:"enable_cors"`
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics"`
	EnableAuth    bool `json:"enable_auth" yaml:"enable_auth"`

	// Rate limiting
	RateLimit int `json:"rate_limit" yaml:"rate_limit"` // requests per second
	RateBurst int `json:"rate_burst" yaml:"rate_burst"` // burst size
}

// TLSConfig contains TLS configuration
type TLSConfig struct {
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	CertFile string `json:"cert_file" yaml:"cert_file"`
	KeyFile  string `json:"key_file" yaml:"key_file"`
	CAFile   string `json:"ca_file" yaml:"ca_file"`
}

// MetricsConfig contains monitoring configuration
type MetricsConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Port    int    `json:"port" yaml:"port"`
	Path    string `json:"path" yaml:"path"`

	// Prometheus settings
	Namespace string `json:"namespace" yaml:"namespace"`
	Subsystem string `json:"subsystem" yaml:"subsystem"`

	// Collection intervals
	CollectInterval time.Duration `json:"collect_interval" yaml:"collect_interval"`

	// Health check endpoints
	HealthPath string `json:"health_path" yaml:"health_path"`
	ReadyPath  string `json:"ready_path" yaml:"ready_path"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level      string `json:"level" yaml:"level"`       // debug, info, warn, error
	Format     string `json:"format" yaml:"format"`     // json, text
	Output     string `json:"output" yaml:"output"`     // stdout, stderr, file
	File       string `json:"file" yaml:"file"`         // log file path
	MaxSize    int    `json:"max_size" yaml:"max_size"` // MB
	MaxBackups int    `json:"max_backups" yaml:"max_backups"`
	MaxAge     int    `json:"max_age" yaml:"max_age"` // days
	Compress   bool   `json:"compress" yaml:"compress"`

	// Structured logging fields
	Fields map[string]interface{} `json:"fields" yaml:"fields"`
}

// CircuitBreakerConfig contains circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled      bool          `json:"enabled" yaml:"enabled"`
	Threshold    int           `json:"threshold" yaml:"threshold"`         // failures before opening
	Timeout      time.Duration `json:"timeout" yaml:"timeout"`             // how long to stay open
	MaxRequests  int           `json:"max_requests" yaml:"max_requests"`   // max requests in half-open
	ResetTimeout time.Duration `json:"reset_timeout" yaml:"reset_timeout"` // time to reset counters

	// Per-service settings
	Services map[string]ServiceCircuitConfig `json:"services" yaml:"services"`
}

// ServiceCircuitConfig contains service-specific circuit breaker settings
type ServiceCircuitConfig struct {
	Threshold   int           `json:"threshold" yaml:"threshold"`
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`
	MaxRequests int           `json:"max_requests" yaml:"max_requests"`
}

// RateLimitConfig contains rate limiting configuration
type RateLimitConfig struct {
	Enabled         bool          `json:"enabled" yaml:"enabled"`
	DefaultLimit    int           `json:"default_limit" yaml:"default_limit"` // requests per second
	DefaultBurst    int           `json:"default_burst" yaml:"default_burst"` // burst size
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`

	// Per-client and per-task-type limits
	ClientLimits   map[string]LimitConfig   `json:"client_limits" yaml:"client_limits"`
	TaskTypeLimits map[TaskType]LimitConfig `json:"task_type_limits" yaml:"task_type_limits"`
}

// LimitConfig contains specific rate limit settings
type LimitConfig struct {
	Limit int `json:"limit" yaml:"limit"` // requests per second
	Burst int `json:"burst" yaml:"burst"` // burst size
}

// SecurityConfig contains security-related configuration
type SecurityConfig struct {
	// Authentication
	Auth AuthConfig `json:"auth" yaml:"auth"`

	// JWT settings
	JWT JWTConfig `json:"jwt" yaml:"jwt"`

	// API keys
	APIKeys APIKeyConfig `json:"api_keys" yaml:"api_keys"`

	// Encryption
	Encryption EncryptionConfig `json:"encryption" yaml:"encryption"`
}

// AuthType represents the authentication method
type AuthType string

const (
	AuthTypeNone   AuthType = "none"
	AuthTypeBasic  AuthType = "basic"
	AuthTypeJWT    AuthType = "jwt"
	AuthTypeAPIKey AuthType = "api_key"
)

// AuthConfig contains authentication configuration
type AuthConfig struct {
	Type     AuthType `json:"type" yaml:"type"`
	Enabled  bool     `json:"enabled" yaml:"enabled"`
	Required bool     `json:"required" yaml:"required"`
}

// JWTConfig contains JWT configuration
type JWTConfig struct {
	SecretKey      string        `json:"secret_key" yaml:"secret_key"`
	ExpirationTime time.Duration `json:"expiration_time" yaml:"expiration_time"`
	Issuer         string        `json:"issuer" yaml:"issuer"`
	Audience       string        `json:"audience" yaml:"audience"`
}

// APIKeyConfig contains API key configuration
type APIKeyConfig struct {
	HeaderName string   `json:"header_name" yaml:"header_name"`
	Keys       []string `json:"keys" yaml:"keys"`
}

// EncryptionConfig contains encryption configuration
type EncryptionConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Algorithm string `json:"algorithm" yaml:"algorithm"` // AES-256-GCM
	KeyFile   string `json:"key_file" yaml:"key_file"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:        "taskforge",
			Version:     "1.0.0",
			Environment: "development",
			Debug:       false,
		},
		Queue: QueueConfig{
			Backend:          "redis",
			URL:              "redis://localhost:6379",
			MaxRetries:       3,
			Timeout:          30 * time.Second,
			DefaultQueue:     "default",
			MaxQueueSize:     10000,
			CompletedTaskTTL: 24 * time.Hour,
			FailedTaskTTL:    7 * 24 * time.Hour,
			Redis: RedisConfig{
				DB:           0,
				MaxRetries:   3,
				PoolSize:     10,
				MinIdleConns: 5,
				DialTimeout:  5 * time.Second,
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				PoolTimeout:  4 * time.Second,
			},
		},
		Worker: WorkerConfig{
			Queues:            []string{"default"},
			Concurrency:       5,
			Timeout:           5 * time.Minute,
			HeartbeatInterval: 30 * time.Second,
			ShutdownTimeout:   30 * time.Second,
			MaxRetries:        3,
			RetryBackoff:      "exponential",
			InitialDelay:      1 * time.Second,
			MaxDelay:          5 * time.Minute,
			BackoffFactor:     2.0,
			MaxMemoryMB:       512,
			MaxCPUPercent:     80,
		},
		Scheduler: SchedulerConfig{
			Enabled:           true,
			CheckInterval:     10 * time.Second,
			MaxScheduledTasks: 1000,
			Timezone:          "UTC",
			CleanupInterval:   1 * time.Hour,
			CleanupAge:        7 * 24 * time.Hour,
		},
		API: APIConfig{
			Enabled:       true,
			Host:          "0.0.0.0",
			Port:          8080,
			ReadTimeout:   30 * time.Second,
			WriteTimeout:  30 * time.Second,
			IdleTimeout:   60 * time.Second,
			EnableCORS:    true,
			EnableMetrics: true,
			EnableAuth:    false,
			RateLimit:     100,
			RateBurst:     10,
		},
		Metrics: MetricsConfig{
			Enabled:         true,
			Port:            9090,
			Path:            "/metrics",
			Namespace:       "taskforge",
			Subsystem:       "",
			CollectInterval: 15 * time.Second,
			HealthPath:      "/health",
			ReadyPath:       "/ready",
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			Output:     "stdout",
			MaxSize:    100,
			MaxBackups: 3,
			MaxAge:     7,
			Compress:   true,
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:      true,
			Threshold:    5,
			Timeout:      60 * time.Second,
			MaxRequests:  3,
			ResetTimeout: 300 * time.Second,
		},
		RateLimit: RateLimitConfig{
			Enabled:         true,
			DefaultLimit:    100,
			DefaultBurst:    10,
			CleanupInterval: 5 * time.Minute,
		},
		Security: SecurityConfig{
			Auth: AuthConfig{
				Type: AuthTypeNone,
			},
			JWT: JWTConfig{
				ExpirationTime: 24 * time.Hour,
				Issuer:         "taskforge",
			},
			APIKeys: APIKeyConfig{
				HeaderName: "X-API-Key",
			},
			Encryption: EncryptionConfig{
				Enabled:   false,
				Algorithm: "AES-256-GCM",
			},
		},
	}
}
