package factory

import (
	"testing"

	"github.com/2bxtech/taskforge/pkg/types"
)

// MockLogger for testing
type MockLogger struct{}

func (m *MockLogger) Debug(msg string, fields ...types.Field) {}
func (m *MockLogger) Info(msg string, fields ...types.Field)  {}
func (m *MockLogger) Warn(msg string, fields ...types.Field)  {}
func (m *MockLogger) Error(msg string, fields ...types.Field) {}
func (m *MockLogger) With(fields ...types.Field) types.Logger { return m }

func TestNewQueueBackendFactory(t *testing.T) {
	factory := NewQueueBackendFactory()
	if factory == nil {
		t.Error("NewQueueBackendFactory() returned nil")
	}
}

func TestQueueBackendFactory_CreateQueueBackend(t *testing.T) {
	factory := NewQueueBackendFactory()
	logger := &MockLogger{}

	tests := []struct {
		name    string
		config  *types.QueueConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "unsupported backend",
			config: &types.QueueConfig{
				Backend: "unsupported",
			},
			wantErr: true,
		},
		{
			name: "postgres backend (not implemented)",
			config: &types.QueueConfig{
				Backend: "postgres",
			},
			wantErr: true,
		},
		{
			name: "nats backend (not implemented)",
			config: &types.QueueConfig{
				Backend: "nats",
			},
			wantErr: true,
		},
		{
			name: "memory backend (not implemented)",
			config: &types.QueueConfig{
				Backend: "memory",
			},
			wantErr: true,
		},
		// Redis test would require actual Redis connection, skip for unit tests
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := factory.CreateQueueBackend(tt.config, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateQueueBackend() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractAddr(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "empty url",
			url:  "",
			want: "localhost:6379",
		},
		{
			name: "simple host:port",
			url:  "localhost:6379",
			want: "localhost:6379",
		},
		{
			name: "redis url without password",
			url:  "redis://localhost:6379",
			want: "localhost:6379",
		},
		{
			name: "redis url with password",
			url:  "redis://password@localhost:6379",
			want: "localhost:6379",
		},
		{
			name: "redis url with database",
			url:  "redis://localhost:6379/0",
			want: "localhost:6379",
		},
		{
			name: "redis url with password and database",
			url:  "redis://password@localhost:6379/0",
			want: "localhost:6379",
		},
		{
			name: "custom port",
			url:  "redis://localhost:6380",
			want: "localhost:6380",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAddr(tt.url); got != tt.want {
				t.Errorf("extractAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewQueueBackendRegistry(t *testing.T) {
	registry := NewQueueBackendRegistry()
	if registry == nil {
		t.Error("NewQueueBackendRegistry() returned nil")
	}

	// Check that redis is registered by default
	available := registry.ListAvailable()
	found := false
	for _, backend := range available {
		if backend == "redis" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Redis backend not registered by default")
	}
}

func TestQueueBackendRegistry_Register(t *testing.T) {
	registry := NewQueueBackendRegistry()

	// Register a mock backend
	registry.Register("mock", func(config *types.QueueConfig, logger types.Logger) (types.QueueBackend, error) {
		return nil, nil
	})

	available := registry.ListAvailable()
	found := false
	for _, backend := range available {
		if backend == "mock" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Mock backend not registered")
	}
}

func TestQueueBackendRegistry_Create(t *testing.T) {
	registry := NewQueueBackendRegistry()
	logger := &MockLogger{}

	tests := []struct {
		name        string
		backendType string
		config      *types.QueueConfig
		wantErr     bool
	}{
		{
			name:        "unknown backend",
			backendType: "unknown",
			config:      &types.QueueConfig{},
			wantErr:     true,
		},
		// Redis test would require actual Redis connection
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := registry.Create(tt.backendType, tt.config, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFIFOStrategy(t *testing.T) {
	strategy := &FIFOStrategy{}
	// Test that we can create the strategy successfully
	// The nil check was flagged as "impossible condition" because
	// struct literals always return non-nil pointers
	t.Logf("FIFOStrategy created successfully: %T", strategy)
	// Note: Testing Enqueue/Dequeue would require a real backend implementation
}

func TestPriorityStrategy(t *testing.T) {
	strategy := &PriorityStrategy{}
	// Test that we can create the strategy successfully
	// The nil check was flagged as "impossible condition" because
	// struct literals always return non-nil pointers
	t.Logf("PriorityStrategy created successfully: %T", strategy)
	// Note: Testing Enqueue/Dequeue would require a real backend implementation
}
