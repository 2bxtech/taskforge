package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/2bxtech/taskforge/pkg/types"
)

// TaskCommand represents a command in the Command pattern for task execution
// This provides a consistent interface for executing different types of tasks
// with proper isolation, timeout handling, and resource management
type TaskCommand interface {
	// Execute processes the task and returns a result
	Execute(ctx context.Context, task *types.Task) (*types.TaskResult, error)

	// GetTaskType returns the task type this command handles
	GetTaskType() types.TaskType

	// GetCapabilities returns additional capabilities this command provides
	GetCapabilities() []string

	// Validate checks if the task can be processed by this command
	Validate(task *types.Task) error

	// GetResourceRequirements returns resource requirements for execution
	GetResourceRequirements() ResourceRequirements

	// SupportsTask checks if this command can handle the given task
	SupportsTask(task *types.Task) bool
}

// ResourceRequirements defines resource requirements for task execution
type ResourceRequirements struct {
	MaxMemoryMB     int           `json:"max_memory_mb"`
	MaxCPUPercent   int           `json:"max_cpu_percent"`
	MaxDuration     time.Duration `json:"max_duration"`
	RequiresGPU     bool          `json:"requires_gpu"`
	RequiresNetwork bool          `json:"requires_network"`
	Priority        int           `json:"priority"` // Higher priority gets more resources
}

// TaskCommandRegistry manages task commands and provides command resolution
// It implements the Registry pattern for pluggable task processors
type TaskCommandRegistry struct {
	commands     map[types.TaskType]TaskCommand
	capabilities map[string][]TaskCommand
	mutex        sync.RWMutex
	logger       types.Logger

	// Default resource limits
	defaultLimits ResourceRequirements
}

// NewTaskCommandRegistry creates a new command registry
func NewTaskCommandRegistry(logger types.Logger) *TaskCommandRegistry {
	return &TaskCommandRegistry{
		commands:     make(map[types.TaskType]TaskCommand),
		capabilities: make(map[string][]TaskCommand),
		logger:       logger,
		defaultLimits: ResourceRequirements{
			MaxMemoryMB:   256,
			MaxCPUPercent: 50,
			MaxDuration:   5 * time.Minute,
		},
	}
}

// RegisterCommand registers a new task command
func (r *TaskCommandRegistry) RegisterCommand(cmd TaskCommand) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	taskType := cmd.GetTaskType()
	if _, exists := r.commands[taskType]; exists {
		return fmt.Errorf("command for task type %s already registered", taskType)
	}

	r.commands[taskType] = cmd

	// Register capabilities
	for _, capability := range cmd.GetCapabilities() {
		r.capabilities[capability] = append(r.capabilities[capability], cmd)
	}

	r.logger.Info("task command registered",
		types.Field{Key: "task_type", Value: taskType},
		types.Field{Key: "capabilities", Value: cmd.GetCapabilities()})

	return nil
}

// UnregisterCommand removes a task command
func (r *TaskCommandRegistry) UnregisterCommand(taskType types.TaskType) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	cmd, exists := r.commands[taskType]
	if !exists {
		return
	}

	delete(r.commands, taskType)

	// Remove from capabilities
	for capability, commands := range r.capabilities {
		for i, c := range commands {
			if c == cmd {
				r.capabilities[capability] = append(commands[:i], commands[i+1:]...)
				break
			}
		}

		// Clean up empty capability lists
		if len(r.capabilities[capability]) == 0 {
			delete(r.capabilities, capability)
		}
	}

	r.logger.Info("task command unregistered", types.Field{Key: "task_type", Value: taskType})
}

// GetCommand retrieves a command for the specified task type
func (r *TaskCommandRegistry) GetCommand(taskType types.TaskType) (TaskCommand, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	cmd, exists := r.commands[taskType]
	if !exists {
		return nil, fmt.Errorf("no command registered for task type: %s", taskType)
	}

	return cmd, nil
}

// GetCommandForTask finds the best command to handle a specific task
func (r *TaskCommandRegistry) GetCommandForTask(task *types.Task) (TaskCommand, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// First try exact task type match
	if cmd, exists := r.commands[task.Type]; exists && cmd.SupportsTask(task) {
		return cmd, nil
	}

	// Search through all commands to find one that supports this task
	for _, cmd := range r.commands {
		if cmd.SupportsTask(task) {
			return cmd, nil
		}
	}

	return nil, fmt.Errorf("no suitable command found for task %s of type %s", task.ID, task.Type)
}

// GetCommandsByCapability returns all commands that support a specific capability
func (r *TaskCommandRegistry) GetCommandsByCapability(capability string) []TaskCommand {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	commands, exists := r.capabilities[capability]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]TaskCommand, len(commands))
	copy(result, commands)
	return result
}

// GetSupportedTypes returns all supported task types
func (r *TaskCommandRegistry) GetSupportedTypes() []types.TaskType {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	types := make([]types.TaskType, 0, len(r.commands))
	for taskType := range r.commands {
		types = append(types, taskType)
	}
	return types
}

// GetAllCapabilities returns all available capabilities
func (r *TaskCommandRegistry) GetAllCapabilities() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	capabilities := make([]string, 0, len(r.capabilities))
	for capability := range r.capabilities {
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

// BaseTaskCommand provides a base implementation for task commands
// It includes common functionality like validation, resource management, and error handling
type BaseTaskCommand struct {
	taskType     types.TaskType
	capabilities []string
	requirements ResourceRequirements
	processor    types.TaskProcessor
	logger       types.Logger

	// Execution tracking
	executionCount int64
	errorCount     int64
	totalDuration  time.Duration
	mutex          sync.RWMutex
}

// NewBaseTaskCommand creates a new base task command
func NewBaseTaskCommand(
	taskType types.TaskType,
	capabilities []string,
	requirements ResourceRequirements,
	processor types.TaskProcessor,
	logger types.Logger,
) *BaseTaskCommand {
	return &BaseTaskCommand{
		taskType:     taskType,
		capabilities: capabilities,
		requirements: requirements,
		processor:    processor,
		logger:       logger,
	}
}

// GetTaskType returns the task type this command handles
func (b *BaseTaskCommand) GetTaskType() types.TaskType {
	return b.taskType
}

// GetCapabilities returns additional capabilities this command provides
func (b *BaseTaskCommand) GetCapabilities() []string {
	return b.capabilities
}

// GetResourceRequirements returns resource requirements for execution
func (b *BaseTaskCommand) GetResourceRequirements() ResourceRequirements {
	return b.requirements
}

// SupportsTask checks if this command can handle the given task
func (b *BaseTaskCommand) SupportsTask(task *types.Task) bool {
	// Check if task type matches
	if task.Type != b.taskType {
		return false
	}

	// Check if processor supports this task type
	supportedTypes := b.processor.GetSupportedTypes()
	for _, supportedType := range supportedTypes {
		if supportedType == task.Type {
			return true
		}
	}

	return false
}

// Validate checks if the task can be processed by this command
func (b *BaseTaskCommand) Validate(task *types.Task) error {
	if task == nil {
		return fmt.Errorf("task cannot be nil")
	}

	if task.Type != b.taskType {
		return fmt.Errorf("task type %s does not match command type %s", task.Type, b.taskType)
	}

	if len(task.Payload) == 0 {
		return fmt.Errorf("task payload cannot be empty")
	}

	// Additional validation can be implemented by subclasses
	return nil
}

// Execute processes the task using the underlying processor
func (b *BaseTaskCommand) Execute(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
	startTime := time.Now()

	// Update execution count
	b.mutex.Lock()
	b.executionCount++
	b.mutex.Unlock()

	// Validate task before processing
	if err := b.Validate(task); err != nil {
		b.recordError()
		return &types.TaskResult{
			TaskID:      task.ID,
			Status:      types.TaskStatusFailed,
			Error:       fmt.Sprintf("validation failed: %v", err),
			Duration:    time.Since(startTime),
			CompletedAt: time.Now(),
			WorkerID:    b.getWorkerID(ctx),
		}, err
	}

	// Set up timeout context if task has timeout
	execCtx := ctx
	if task.Timeout != nil {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, *task.Timeout)
		defer cancel()
	}

	// Execute the task
	result, err := b.processor.Process(execCtx, task)
	duration := time.Since(startTime)

	// Update statistics
	b.mutex.Lock()
	b.totalDuration += duration
	if err != nil {
		b.errorCount++
	}
	b.mutex.Unlock()

	// Ensure result has required fields
	if result == nil {
		result = &types.TaskResult{
			TaskID:   task.ID,
			WorkerID: b.getWorkerID(ctx),
		}
	}

	result.Duration = duration
	result.CompletedAt = time.Now()

	if err != nil {
		result.Status = types.TaskStatusFailed
		result.Error = err.Error()

		b.logger.Error("task command execution failed",
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "task_type", Value: task.Type},
			types.Field{Key: "duration", Value: duration},
			types.Field{Key: "error", Value: err.Error()})
	} else {
		result.Status = types.TaskStatusCompleted

		b.logger.Debug("task command executed successfully",
			types.Field{Key: "task_id", Value: task.ID},
			types.Field{Key: "task_type", Value: task.Type},
			types.Field{Key: "duration", Value: duration})
	}

	return result, err
}

// recordError increments the error count
func (b *BaseTaskCommand) recordError() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.errorCount++
}

// getWorkerID extracts worker ID from context
func (b *BaseTaskCommand) getWorkerID(ctx context.Context) string {
	if workerID, ok := ctx.Value(WorkerIDKey).(string); ok {
		return workerID
	}
	return "unknown"
}

// GetStats returns execution statistics for this command
func (b *BaseTaskCommand) GetStats() CommandStats {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	var avgDuration time.Duration
	if b.executionCount > 0 {
		avgDuration = time.Duration(b.totalDuration.Nanoseconds() / b.executionCount)
	}

	var errorRate float64
	if b.executionCount > 0 {
		errorRate = float64(b.errorCount) / float64(b.executionCount)
	}

	return CommandStats{
		TaskType:       b.taskType,
		ExecutionCount: b.executionCount,
		ErrorCount:     b.errorCount,
		ErrorRate:      errorRate,
		TotalDuration:  b.totalDuration,
		AvgDuration:    avgDuration,
	}
}

// CommandStats contains execution statistics for a command
type CommandStats struct {
	TaskType       types.TaskType `json:"task_type"`
	ExecutionCount int64          `json:"execution_count"`
	ErrorCount     int64          `json:"error_count"`
	ErrorRate      float64        `json:"error_rate"`
	TotalDuration  time.Duration  `json:"total_duration"`
	AvgDuration    time.Duration  `json:"avg_duration"`
}

// IsolatedTaskCommand wraps a task command with additional isolation features
// It implements the Decorator pattern to add isolation capabilities
type IsolatedTaskCommand struct {
	TaskCommand

	// Isolation features
	resourceLimiter ResourceLimiter
	circuitBreaker  types.CircuitBreaker
	rateLimiter     types.RateLimiter
	logger          types.Logger

	// Isolation configuration
	enableResourceLimits bool
	enableCircuitBreaker bool
	enableRateLimit      bool

	// Statistics
	isolationViolations int64
	rateLimitHits       int64
	circuitBreakerHits  int64
	mutex               sync.RWMutex
}

// ResourceLimiter defines interface for resource limiting
type ResourceLimiter interface {
	// AcquireResources attempts to acquire resources for task execution
	AcquireResources(ctx context.Context, requirements ResourceRequirements) (ResourceToken, error)

	// GetAvailableResources returns currently available resources
	GetAvailableResources() AvailableResources
}

// ResourceToken represents acquired resources that must be released
type ResourceToken interface {
	// Release returns the acquired resources
	Release()

	// GetAcquiredResources returns the resources acquired by this token
	GetAcquiredResources() ResourceRequirements
}

// AvailableResources represents currently available system resources
type AvailableResources struct {
	MemoryMB    int `json:"memory_mb"`
	CPUPercent  int `json:"cpu_percent"`
	ActiveTasks int `json:"active_tasks"`
	MaxTasks    int `json:"max_tasks"`
}

// NewIsolatedTaskCommand creates a new isolated task command
func NewIsolatedTaskCommand(
	baseCommand TaskCommand,
	resourceLimiter ResourceLimiter,
	circuitBreaker types.CircuitBreaker,
	rateLimiter types.RateLimiter,
	logger types.Logger,
) *IsolatedTaskCommand {
	return &IsolatedTaskCommand{
		TaskCommand:          baseCommand,
		resourceLimiter:      resourceLimiter,
		circuitBreaker:       circuitBreaker,
		rateLimiter:          rateLimiter,
		logger:               logger,
		enableResourceLimits: resourceLimiter != nil,
		enableCircuitBreaker: circuitBreaker != nil,
		enableRateLimit:      rateLimiter != nil,
	}
}

// Execute processes the task with isolation features
func (i *IsolatedTaskCommand) Execute(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
	// Check rate limit first
	if i.enableRateLimit {
		allowed, err := i.rateLimiter.Allow(ctx, string(i.GetTaskType()))
		if err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}
		if !allowed {
			i.mutex.Lock()
			i.rateLimitHits++
			i.mutex.Unlock()

			return &types.TaskResult{
				TaskID:   task.ID,
				Status:   types.TaskStatusFailed,
				Error:    "rate limit exceeded",
				WorkerID: i.getWorkerID(ctx),
			}, fmt.Errorf("rate limit exceeded for task type %s", i.GetTaskType())
		}
	}

	// Execute with circuit breaker protection
	if i.enableCircuitBreaker {
		var result *types.TaskResult
		var execErr error

		cbErr := i.circuitBreaker.Execute(func() error {
			var err error
			result, err = i.executeWithResourceLimits(ctx, task)
			execErr = err
			return err
		})

		if cbErr != nil {
			i.mutex.Lock()
			i.circuitBreakerHits++
			i.mutex.Unlock()

			return &types.TaskResult{
				TaskID:   task.ID,
				Status:   types.TaskStatusFailed,
				Error:    "circuit breaker open",
				WorkerID: i.getWorkerID(ctx),
			}, fmt.Errorf("circuit breaker open for task type %s: %w", i.GetTaskType(), cbErr)
		}

		return result, execErr
	}

	// Execute without circuit breaker
	return i.executeWithResourceLimits(ctx, task)
}

// executeWithResourceLimits executes the task with resource limit enforcement
func (i *IsolatedTaskCommand) executeWithResourceLimits(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
	// Acquire resources if resource limiting is enabled
	if i.enableResourceLimits {
		requirements := i.GetResourceRequirements()

		token, err := i.resourceLimiter.AcquireResources(ctx, requirements)
		if err != nil {
			i.mutex.Lock()
			i.isolationViolations++
			i.mutex.Unlock()

			return &types.TaskResult{
				TaskID:   task.ID,
				Status:   types.TaskStatusFailed,
				Error:    "insufficient resources",
				WorkerID: i.getWorkerID(ctx),
			}, fmt.Errorf("failed to acquire resources: %w", err)
		}

		defer token.Release()
	}

	// Execute the underlying command
	return i.TaskCommand.Execute(ctx, task)
}

// getWorkerID extracts worker ID from context
func (i *IsolatedTaskCommand) getWorkerID(ctx context.Context) string {
	if workerID, ok := ctx.Value(WorkerIDKey).(string); ok {
		return workerID
	}
	return "unknown"
}

// GetIsolationStats returns isolation-related statistics
func (i *IsolatedTaskCommand) GetIsolationStats() IsolationStats {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	return IsolationStats{
		TaskType:              i.GetTaskType(),
		IsolationViolations:   i.isolationViolations,
		RateLimitHits:         i.rateLimitHits,
		CircuitBreakerHits:    i.circuitBreakerHits,
		ResourceLimitsEnabled: i.enableResourceLimits,
		CircuitBreakerEnabled: i.enableCircuitBreaker,
		RateLimitEnabled:      i.enableRateLimit,
	}
}

// IsolationStats contains isolation-related statistics
type IsolationStats struct {
	TaskType              types.TaskType `json:"task_type"`
	IsolationViolations   int64          `json:"isolation_violations"`
	RateLimitHits         int64          `json:"rate_limit_hits"`
	CircuitBreakerHits    int64          `json:"circuit_breaker_hits"`
	ResourceLimitsEnabled bool           `json:"resource_limits_enabled"`
	CircuitBreakerEnabled bool           `json:"circuit_breaker_enabled"`
	RateLimitEnabled      bool           `json:"rate_limit_enabled"`
}
