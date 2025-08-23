package redis

import (
	"encoding/json"
	"fmt"

	"github.com/2bxtech/taskforge/pkg/types"
)

// TaskSerializer handles task serialization and deserialization
type TaskSerializer struct{}

// NewTaskSerializer creates a new task serializer
func NewTaskSerializer() *TaskSerializer {
	return &TaskSerializer{}
}

// Serialize converts a task to JSON bytes
func (s *TaskSerializer) Serialize(task *types.Task) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	data, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}

	return data, nil
}

// Deserialize converts JSON bytes back to a task
func (s *TaskSerializer) Deserialize(data []byte) (*types.Task, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty task data")
	}

	var task types.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task: %w", err)
	}

	return &task, nil
}

// SerializeBatch converts multiple tasks to JSON bytes
func (s *TaskSerializer) SerializeBatch(tasks []*types.Task) ([][]byte, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	result := make([][]byte, len(tasks))
	for i, task := range tasks {
		data, err := s.Serialize(task)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize task at index %d: %w", i, err)
		}
		result[i] = data
	}

	return result, nil
}

// DeserializeBatch converts multiple JSON bytes back to tasks
func (s *TaskSerializer) DeserializeBatch(dataList [][]byte) ([]*types.Task, error) {
	if len(dataList) == 0 {
		return nil, nil
	}

	result := make([]*types.Task, len(dataList))
	for i, data := range dataList {
		task, err := s.Deserialize(data)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize task at index %d: %w", i, err)
		}
		result[i] = task
	}

	return result, nil
}

// ValidateTask performs basic validation on a task before serialization
func (s *TaskSerializer) ValidateTask(task *types.Task) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	if task.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	if task.Type == "" {
		return fmt.Errorf("task type is required")
	}

	// Set defaults before validation
	if task.Priority == "" {
		task.Priority = types.PriorityNormal
	}

	if task.Status == "" {
		task.Status = types.TaskStatusPending
	}

	// Validate task type
	validTypes := map[types.TaskType]bool{
		types.TaskTypeWebhook:      true,
		types.TaskTypeEmail:        true,
		types.TaskTypeImageProcess: true,
		types.TaskTypeDataProcess:  true,
		types.TaskTypeScheduled:    true,
		types.TaskTypeBatch:        true,
	}

	if !validTypes[task.Type] {
		return fmt.Errorf("invalid task type: %s", task.Type)
	}

	// Validate priority after default is set
	validPriorities := map[types.Priority]bool{
		types.PriorityCritical: true,
		types.PriorityHigh:     true,
		types.PriorityNormal:   true,
		types.PriorityLow:      true,
	}

	if !validPriorities[task.Priority] {
		return fmt.Errorf("invalid priority '%s': must be one of: critical, high, normal, low", task.Priority)
	}

	// Validate status
	validStatuses := map[types.TaskStatus]bool{
		types.TaskStatusPending:    true,
		types.TaskStatusRunning:    true,
		types.TaskStatusCompleted:  true,
		types.TaskStatusFailed:     true,
		types.TaskStatusDeadLetter: true,
		types.TaskStatusCancelled:  true,
	}

	if !validStatuses[task.Status] {
		return fmt.Errorf("invalid status: %s", task.Status)
	}

	return nil
}
