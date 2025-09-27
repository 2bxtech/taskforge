# 🔨 TaskForge

[![CI](https://img.shields.io/github/actions/workflow/status/2bxtech/taskforge/ci.yml?branch=main&label=build)](https://github.com/2bxtech/taskforge/actions/workflows/ci.yml)
[![Integration](https://img.shields.io/github/actions/workflow/status/2bxtech/taskforge/integration.yml?branch=main&label=integration)](https://github.com/2bxtech/taskforge/actions/workflows/integration.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/2bxtech/taskforge)](https://goreportcard.com/report/github.com/2bxtech/taskforge)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A distributed task queue system built with Go, demonstrating distributed systems patterns and fault-tolerant design.

## ✨ What TaskForge Shows

**Distributed Systems Implementation**
- Redis Streams with consumer groups for message delivery
- Worker engine with Observer, Command, and Bulkhead patterns
- Circuit breakers and rate limiting for fault tolerance
- Exponential backoff retry with jitter and dead letter queues

**Go Code Practices**
- SOLID principles with dependency injection
- Interface-driven design for pluggable components
- Error handling and structured logging
- Concurrent processing with resource management

**Design Patterns**
- Factory and Strategy patterns for extensibility  
- Observer pattern for event monitoring
- Bulkhead isolation to prevent cascade failures
- Priority queues with FIFO ordering within levels

## 🚀 Quick Demo

```bash
# Clone and setup
git clone https://github.com/2bxtech/taskforge.git
cd taskforge
go mod tidy

# Start Redis (requires Docker)
docker run -d -p 6379:6379 redis:7-alpine

# Try the demos
make redis-demo          # Redis queue backend demo
make worker-demo         # Complete worker engine demo
make demo-all           # Run both demos
```

## �️ Architecture Overview

### Core Components
- **Queue Backend**: Redis Streams implementation with consumer groups
- **Worker Engine**: Task processing with resource management  
- **Task System**: 6 task types (webhook, email, image, data, scheduled, batch)
- **Observability**: Event monitoring with health tracking

### Features
- Consumer groups, retries, dead letter queues
- Batch operations, connection pooling, priority scheduling
- Circuit breakers, bulkhead isolation, graceful degradation
- Health checks, metrics collection, event tracking

## 📊 Project Status

**Phase 1 Complete**: Core architecture and type system  
**Phase 2A Complete**: Redis Queue Backend with consumer groups  
**Phase 2B Complete**: Worker Engine with fault tolerance patterns

**Built and tested** - Functional distributed task queue system.

## 🔧 Development

```bash
# Build all components
make build

# Run tests
make test

# Run integration tests (requires Redis)
make test-integration

# Try the demos
make redis-demo        # Redis queue backend demo  
make worker-demo       # Worker engine with fault tolerance
make demo-all         # Run both demos sequentially
```

## 📁 Project Structure

```
├── pkg/types/              # Public interfaces and types
├── internal/queue/         # Redis queue implementation
├── internal/worker/        # Worker engine with fault tolerance
├── cmd/                    # Application entry points
├── examples/              # Working demonstrations
└── tests/integration/     # Integration test suite
```

## 🎯 Technical Implementation

- **Go 1.22** with concurrency patterns
- **Redis Streams** for distributed queuing  
- **Interface-driven design** with pluggable backends
- **Testing** with race detection and coverage
- **Working demos** showing functionality

---

*A distributed task queue system built with Go and Redis.*
