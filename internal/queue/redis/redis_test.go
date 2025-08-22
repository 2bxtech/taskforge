package redis

import (
	"context"
	"testing"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// MockLogger implements types.Logger for testing
type MockLogger struct {
	logs []LogEntry
}

type LogEntry struct {
	Level   string
	Message string
	Fields  []types.Field
}

func NewMockLogger() *MockLogger {
	return &MockLogger{
		logs: make([]LogEntry, 0),
	}
}

func (m *MockLogger) Debug(msg string, fields ...types.Field) {
	m.logs = append(m.logs, LogEntry{Level: "debug", Message: msg, Fields: fields})
}

func (m *MockLogger) Info(msg string, fields ...types.Field) {
	m.logs = append(m.logs, LogEntry{Level: "info", Message: msg, Fields: fields})
}

func (m *MockLogger) Warn(msg string, fields ...types.Field) {
	m.logs = append(m.logs, LogEntry{Level: "warn", Message: msg, Fields: fields})
}

func (m *MockLogger) Error(msg string, fields ...types.Field) {
	m.logs = append(m.logs, LogEntry{Level: "error", Message: msg, Fields: fields})
}

func (m *MockLogger) With(fields ...types.Field) types.Logger {
	return m // Simplified for testing
}

func (m *MockLogger) GetLogs() []LogEntry {
	return m.logs
}

func TestRedisConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing addr",
			config: &Config{
				Addr:          "",
				PoolSize:      10,
				StreamPrefix:  "test:",
				ConsumerGroup: "test-group",
				DialTimeout:   5 * time.Second,
				ReadTimeout:   3 * time.Second,
				WriteTimeout:  3 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid pool size",
			config: &Config{
				Addr:          "localhost:6379",
				PoolSize:      0,
				StreamPrefix:  "test:",
				ConsumerGroup: "test-group",
				DialTimeout:   5 * time.Second,
				ReadTimeout:   3 * time.Second,
				WriteTimeout:  3 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid min idle conns",
			config: &Config{
				Addr:          "localhost:6379",
				PoolSize:      10,
				MinIdleConns:  15, // Greater than pool size
				StreamPrefix:  "test:",
				ConsumerGroup: "test-group",
				DialTimeout:   5 * time.Second,
				ReadTimeout:   3 * time.Second,
				WriteTimeout:  3 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if err := config.Validate(); err != nil {
		t.Errorf("DefaultConfig() should be valid, got error: %v", err)
	}

	// Test specific default values
	if config.Addr != "localhost:6379" {
		t.Errorf("Expected default addr 'localhost:6379', got '%s'", config.Addr)
	}

	if config.PoolSize != 10 {
		t.Errorf("Expected default pool size 10, got %d", config.PoolSize)
	}

	if config.StreamPrefix != "taskforge:stream:" {
		t.Errorf("Expected default stream prefix 'taskforge:stream:', got '%s'", config.StreamPrefix)
	}
}

func TestConfig_MergeWithDefaults(t *testing.T) {
	partial := &Config{
		Addr:     "redis://custom:6380",
		PoolSize: 20,
	}

	merged := partial.MergeWithDefaults()

	// Should keep custom values
	if merged.Addr != "redis://custom:6380" {
		t.Errorf("Expected custom addr to be preserved, got '%s'", merged.Addr)
	}

	if merged.PoolSize != 20 {
		t.Errorf("Expected custom pool size to be preserved, got %d", merged.PoolSize)
	}

	// Should fill in defaults for missing values
	if merged.StreamPrefix == "" {
		t.Errorf("Expected stream prefix to be filled from defaults")
	}

	if merged.ConsumerGroup == "" {
		t.Errorf("Expected consumer group to be filled from defaults")
	}
}

func TestConfig_GetMethods(t *testing.T) {
	config := DefaultConfig()

	streamName := config.GetStreamName("test-queue")
	expected := config.StreamPrefix + "test-queue"
	if streamName != expected {
		t.Errorf("GetStreamName() = %s, want %s", streamName, expected)
	}

	prioritySetName := config.GetPrioritySetName("test-queue")
	expected = config.PrioritySetPrefix + "test-queue"
	if prioritySetName != expected {
		t.Errorf("GetPrioritySetName() = %s, want %s", prioritySetName, expected)
	}

	dlqStreamName := config.GetDLQStreamName("test-queue")
	expected = config.StreamPrefix + "test-queue" + config.DLQSuffix
	if dlqStreamName != expected {
		t.Errorf("GetDLQStreamName() = %s, want %s", dlqStreamName, expected)
	}

	taskHashKey := config.GetTaskHashKey("task-123")
	expected = config.TaskHashPrefix + "task-123"
	if taskHashKey != expected {
		t.Errorf("GetTaskHashKey() = %s, want %s", taskHashKey, expected)
	}

	consumerName := config.GetConsumerName("worker-1")
	expected = config.ConsumerPrefix + "worker-1"
	if consumerName != expected {
		t.Errorf("GetConsumerName() = %s, want %s", consumerName, expected)
	}
}

func TestTaskSerializer(t *testing.T) {
	serializer := NewTaskSerializer()

	// Create a test task
	task := &types.Task{
		ID:         "test-task-123",
		Type:       types.TaskTypeWebhook,
		Priority:   types.PriorityHigh,
		Status:     types.TaskStatusPending,
		Queue:      "test-queue",
		CreatedAt:  time.Now(),
		MaxRetries: 3,
		Metadata:   map[string]interface{}{"key": "value"},
	}

	// Test serialization
	data, err := serializer.Serialize(task)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialize() returned empty data")
	}

	// Test deserialization
	deserializedTask, err := serializer.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize() error = %v", err)
	}

	// Verify task fields
	if deserializedTask.ID != task.ID {
		t.Errorf("ID mismatch: got %s, want %s", deserializedTask.ID, task.ID)
	}

	if deserializedTask.Type != task.Type {
		t.Errorf("Type mismatch: got %s, want %s", deserializedTask.Type, task.Type)
	}

	if deserializedTask.Priority != task.Priority {
		t.Errorf("Priority mismatch: got %s, want %s", deserializedTask.Priority, task.Priority)
	}

	if deserializedTask.Status != task.Status {
		t.Errorf("Status mismatch: got %s, want %s", deserializedTask.Status, task.Status)
	}
}

func TestTaskSerializer_ValidateTask(t *testing.T) {
	serializer := NewTaskSerializer()

	tests := []struct {
		name    string
		task    *types.Task
		wantErr bool
	}{
		{
			name: "valid task",
			task: &types.Task{
				ID:       "test-123",
				Type:     types.TaskTypeWebhook,
				Priority: types.PriorityHigh,
				Status:   types.TaskStatusPending,
			},
			wantErr: false,
		},
		{
			name:    "nil task",
			task:    nil,
			wantErr: true,
		},
		{
			name: "missing ID",
			task: &types.Task{
				Type:     types.TaskTypeWebhook,
				Priority: types.PriorityHigh,
				Status:   types.TaskStatusPending,
			},
			wantErr: true,
		},
		{
			name: "missing type",
			task: &types.Task{
				ID:       "test-123",
				Priority: types.PriorityHigh,
				Status:   types.TaskStatusPending,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			task: &types.Task{
				ID:       "test-123",
				Type:     "invalid-type",
				Priority: types.PriorityHigh,
				Status:   types.TaskStatusPending,
			},
			wantErr: true,
		},
		{
			name: "invalid priority",
			task: &types.Task{
				ID:       "test-123",
				Type:     types.TaskTypeWebhook,
				Priority: "invalid-priority",
				Status:   types.TaskStatusPending,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := serializer.ValidateTask(tt.task)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTask() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTaskSerializer_Batch(t *testing.T) {
	serializer := NewTaskSerializer()

	// Create test tasks
	tasks := []*types.Task{
		{
			ID:       "task-1",
			Type:     types.TaskTypeWebhook,
			Priority: types.PriorityHigh,
			Status:   types.TaskStatusPending,
		},
		{
			ID:       "task-2",
			Type:     types.TaskTypeEmail,
			Priority: types.PriorityNormal,
			Status:   types.TaskStatusPending,
		},
	}

	// Test batch serialization
	dataList, err := serializer.SerializeBatch(tasks)
	if err != nil {
		t.Fatalf("SerializeBatch() error = %v", err)
	}

	if len(dataList) != len(tasks) {
		t.Errorf("SerializeBatch() returned %d items, want %d", len(dataList), len(tasks))
	}

	// Test batch deserialization
	deserializedTasks, err := serializer.DeserializeBatch(dataList)
	if err != nil {
		t.Fatalf("DeserializeBatch() error = %v", err)
	}

	if len(deserializedTasks) != len(tasks) {
		t.Errorf("DeserializeBatch() returned %d tasks, want %d", len(deserializedTasks), len(tasks))
	}

	// Verify tasks
	for i, task := range deserializedTasks {
		if task.ID != tasks[i].ID {
			t.Errorf("Task %d ID mismatch: got %s, want %s", i, task.ID, tasks[i].ID)
		}
		if task.Type != tasks[i].Type {
			t.Errorf("Task %d Type mismatch: got %s, want %s", i, task.Type, tasks[i].Type)
		}
	}
}

// Integration test helpers (these would require a real Redis instance)
func TestConnectionManager_Integration(t *testing.T) {
	// Skip if Redis is not available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := DefaultConfig()

	// This test would require a real Redis instance
	connMgr, err := NewConnectionManager(config)
	if err != nil {
		t.Skipf("Redis not available for integration test: %v", err)
	}
	defer connMgr.Close()

	ctx := context.Background()

	// Test ping
	if err := connMgr.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}

	// Test health check
	if err := connMgr.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}

	// Test stats
	stats := connMgr.GetStats()
	if stats == nil {
		t.Error("GetStats() returned nil")
	}
}

// Benchmark tests
func BenchmarkTaskSerializer_Serialize(b *testing.B) {
	serializer := NewTaskSerializer()
	task := &types.Task{
		ID:         "benchmark-task",
		Type:       types.TaskTypeWebhook,
		Priority:   types.PriorityHigh,
		Status:     types.TaskStatusPending,
		Queue:      "benchmark-queue",
		CreatedAt:  time.Now(),
		MaxRetries: 3,
		Metadata:   map[string]interface{}{"key": "value"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := serializer.Serialize(task)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTaskSerializer_Deserialize(b *testing.B) {
	serializer := NewTaskSerializer()
	task := &types.Task{
		ID:         "benchmark-task",
		Type:       types.TaskTypeWebhook,
		Priority:   types.PriorityHigh,
		Status:     types.TaskStatusPending,
		Queue:      "benchmark-queue",
		CreatedAt:  time.Now(),
		MaxRetries: 3,
		Metadata:   map[string]interface{}{"key": "value"},
	}

	data, err := serializer.Serialize(task)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := serializer.Deserialize(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
