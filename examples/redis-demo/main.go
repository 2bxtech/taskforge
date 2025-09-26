package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/2bxtech/taskforge/internal/queue/factory"
	"github.com/2bxtech/taskforge/internal/queue/redis"
	"github.com/2bxtech/taskforge/pkg/types"
	"github.com/google/uuid"
)

// SimpleLogger implements types.Logger for the example
type SimpleLogger struct{}

func (l *SimpleLogger) Debug(msg string, fields ...types.Field) {
	fmt.Printf("[DEBUG] %s %v\n", msg, fields)
}

func (l *SimpleLogger) Info(msg string, fields ...types.Field) {
	fmt.Printf("[INFO] %s %v\n", msg, fields)
}

func (l *SimpleLogger) Warn(msg string, fields ...types.Field) {
	fmt.Printf("[WARN] %s %v\n", msg, fields)
}

func (l *SimpleLogger) Error(msg string, fields ...types.Field) {
	fmt.Printf("[ERROR] %s %v\n", msg, fields)
}

func (l *SimpleLogger) With(fields ...types.Field) types.Logger {
	return l
}

func main() {
	// Create logger
	logger := &SimpleLogger{}

	// Create Redis configuration
	redisConfig := redis.DefaultConfig()
	redisConfig.Addr = "localhost:6379" // Make sure Redis is running

	fmt.Println("🔨 TaskForge Redis Queue Backend Demo")
	fmt.Println("=====================================")

	// Method 1: Direct Redis queue creation
	fmt.Println("\n1️⃣ Creating Redis queue directly...")
	redisQueue, err := redis.NewRedisQueue(redisConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create Redis queue: %v", err)
	}
	defer redisQueue.Close()

	// Test connection
	ctx := context.Background()
	if err := redisQueue.HealthCheck(ctx); err != nil {
		log.Printf("⚠️  Redis health check failed (make sure Redis is running): %v", err)
		return
	}
	fmt.Println("✅ Redis connection successful!")

	// Method 2: Using Factory pattern
	fmt.Println("\n2️⃣ Creating queue using Factory pattern...")

	queueConfig := &types.QueueConfig{
		Backend: "redis",
		URL:     "redis://localhost:6379",
		Redis: types.RedisConfig{
			DB:           0,
			PoolSize:     10,
			MinIdleConns: 5,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolTimeout:  4 * time.Second,
		},
	}

	queueFactory := factory.NewQueueBackendFactory()
	queue, err := queueFactory.CreateQueueBackend(queueConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create queue via factory: %v", err)
	}
	defer queue.Close()

	fmt.Println("✅ Queue created via factory!")

	// Method 3: Using Registry pattern
	fmt.Println("\n3️⃣ Using Registry pattern...")

	registry := factory.NewQueueBackendRegistry()
	registryQueue, err := registry.Create("redis", queueConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create queue via registry: %v", err)
	}
	defer registryQueue.Close()

	fmt.Println("✅ Queue created via registry!")
	fmt.Printf("📋 Available backends: %v\n", registry.ListAvailable())

	// Demonstrate queue operations
	fmt.Println("\n4️⃣ Demonstrating queue operations...")

	// Create sample tasks with different priorities
	tasks := []*types.Task{
		{
			ID:         uuid.New().String(),
			Type:       types.TaskTypeWebhook,
			Priority:   types.PriorityCritical,
			Status:     types.TaskStatusPending,
			Queue:      "webhooks",
			CreatedAt:  time.Now(),
			MaxRetries: 3,
			Payload:    []byte(`{"url": "https://api.example.com/webhook", "method": "POST"}`),
			Metadata:   map[string]interface{}{"source": "user_signup"},
		},
		{
			ID:         uuid.New().String(),
			Type:       types.TaskTypeEmail,
			Priority:   types.PriorityHigh,
			Status:     types.TaskStatusPending,
			Queue:      "emails",
			CreatedAt:  time.Now(),
			MaxRetries: 3,
			Payload:    []byte(`{"to": "user@example.com", "template": "welcome"}`),
			Metadata:   map[string]interface{}{"campaign": "welcome_series"},
		},
		{
			ID:         uuid.New().String(),
			Type:       types.TaskTypeImageProcess,
			Priority:   types.PriorityNormal,
			Status:     types.TaskStatusPending,
			Queue:      "images",
			CreatedAt:  time.Now(),
			MaxRetries: 3,
			Payload:    []byte(`{"image_url": "https://example.com/image.jpg", "operations": ["resize", "watermark"]}`),
			Metadata:   map[string]interface{}{"user_id": "12345"},
		},
		{
			ID:         uuid.New().String(),
			Type:       types.TaskTypeDataProcess,
			Priority:   types.PriorityLow,
			Status:     types.TaskStatusPending,
			Queue:      "analytics",
			CreatedAt:  time.Now(),
			MaxRetries: 3,
			Payload:    []byte(`{"data_source": "user_events", "date_range": "2024-01-01:2024-01-31"}`),
			Metadata:   map[string]interface{}{"report_type": "monthly"},
		},
	}

	// Enqueue individual tasks
	fmt.Println("\n📤 Enqueuing individual tasks...")
	for i, task := range tasks {
		if err := queue.Enqueue(ctx, task); err != nil {
			log.Printf("Failed to enqueue task %d: %v", i+1, err)
		} else {
			fmt.Printf("✅ Enqueued %s task (Priority: %s, Queue: %s)\n",
				task.Type, task.Priority, task.Queue)
		}
	}

	// Enqueue batch
	fmt.Println("\n📦 Demonstrating batch enqueue...")
	batchTasks := []*types.Task{
		{
			ID:         uuid.New().String(),
			Type:       types.TaskTypeBatch,
			Priority:   types.PriorityNormal,
			Status:     types.TaskStatusPending,
			Queue:      "batch",
			CreatedAt:  time.Now(),
			MaxRetries: 3,
			Payload:    []byte(`{"batch_id": "batch_001", "items": 100}`),
		},
		{
			ID:         uuid.New().String(),
			Type:       types.TaskTypeBatch,
			Priority:   types.PriorityNormal,
			Status:     types.TaskStatusPending,
			Queue:      "batch",
			CreatedAt:  time.Now(),
			MaxRetries: 3,
			Payload:    []byte(`{"batch_id": "batch_002", "items": 200}`),
		},
	}

	if err := queue.EnqueueBatch(ctx, batchTasks); err != nil {
		log.Printf("Failed to enqueue batch: %v", err)
	} else {
		fmt.Printf("✅ Enqueued batch of %d tasks\n", len(batchTasks))
	}

	// Dequeue tasks from different queues
	fmt.Println("\n📥 Dequeuing tasks...")
	queues := []string{"webhooks", "emails", "images", "analytics", "batch"}

	for _, queueName := range queues {
		task, err := queue.Dequeue(ctx, queueName, 1*time.Second)
		if err != nil {
			log.Printf("Failed to dequeue from %s: %v", queueName, err)
			continue
		}

		if task == nil {
			fmt.Printf("⭕ No tasks in queue: %s\n", queueName)
			continue
		}

		fmt.Printf("✅ Dequeued task from %s: %s (Type: %s, Priority: %s)\n",
			queueName, task.ID, task.Type, task.Priority)

		// Simulate task processing
		time.Sleep(100 * time.Millisecond)

		// Acknowledge successful completion
		if err := queue.Ack(ctx, task.ID); err != nil {
			log.Printf("Failed to ack task %s: %v", task.ID, err)
		} else {
			fmt.Printf("✅ Task %s acknowledged\n", task.ID)
		}
	}

	// Demonstrate queue statistics
	fmt.Println("\n📊 Queue statistics...")
	for _, queueName := range queues {
		stats, err := queue.GetQueueStats(ctx, queueName)
		if err != nil {
			log.Printf("Failed to get stats for %s: %v", queueName, err)
			continue
		}

		fmt.Printf("📈 Queue %s: %d pending, %d running\n",
			stats.QueueName, stats.PendingTasks, stats.RunningTasks)
	}

	// List all queues
	fmt.Println("\n📋 Listing all queues...")
	allQueues, err := queue.ListQueues(ctx)
	if err != nil {
		log.Printf("Failed to list queues: %v", err)
	} else {
		fmt.Printf("📝 Found queues: %v\n", allQueues)
	}

	// Demonstrate error handling and retries
	fmt.Println("\n⚠️  Demonstrating error handling...")

	failingTask := &types.Task{
		ID:         uuid.New().String(),
		Type:       types.TaskTypeWebhook,
		Priority:   types.PriorityHigh,
		Status:     types.TaskStatusPending,
		Queue:      "failing",
		CreatedAt:  time.Now(),
		MaxRetries: 2,
		Payload:    []byte(`{"url": "https://invalid-endpoint.com/webhook"}`),
	}

	if err := queue.Enqueue(ctx, failingTask); err != nil {
		log.Printf("Failed to enqueue failing task: %v", err)
	} else {
		fmt.Printf("✅ Enqueued task that will fail: %s\n", failingTask.ID)

		// Dequeue and simulate failure
		task, err := queue.Dequeue(ctx, "failing", 1*time.Second)
		if err == nil && task != nil {
			fmt.Printf("📥 Dequeued failing task: %s\n", task.ID)

			// Simulate failure
			if err := queue.Nack(ctx, task.ID, "simulated network error"); err != nil {
				log.Printf("Failed to nack task: %v", err)
			} else {
				fmt.Printf("❌ Task failed and will be retried\n")
			}
		}
	}

	fmt.Println("\n🎉 Demo completed successfully!")
	fmt.Println("\n🔍 Key Features Demonstrated:")
	fmt.Println("   ✅ Factory Pattern for queue creation")
	fmt.Println("   ✅ Strategy Pattern for pluggable backends")
	fmt.Println("   ✅ Dependency Injection with interfaces")
	fmt.Println("   ✅ Priority-based task processing")
	fmt.Println("   ✅ Batch operations for performance")
	fmt.Println("   ✅ Comprehensive error handling")
	fmt.Println("   ✅ Redis Streams for reliable delivery")
	fmt.Println("   ✅ Consumer group management")
	fmt.Println("   ✅ Production-ready patterns")
}
