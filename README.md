# 🔨 TaskForge

[![CI](https://img.shields.io/github/actions/workflow/status/2bxtech/taskforge/ci.yml?branch=main&label=build)](https://github.com/2bxtech/taskforge/actions/workflows/ci.yml)
[![Integration](https://img.shields.io/github/actions/workflow/status/2bxtech/taskforge/integration.yml?branch=main&label=integration)](https://github.com/2bxtech/taskforge/actions/workflows/integration.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/2bxtech/taskforge)](https://goreportcard.com/report/github.com/2bxtech/taskforge)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A distributed task queue system built with Go, demonstrating enterprise-grade patterns and distributed systems architecture.

## 🏗️ Current Status

**Phase 1 Complete**: Core architecture and type system
- Complete type definitions for task management
- Interface design for queue backends and workers
- 6 task types: webhooks, email, image processing, data processing, scheduled tasks, batch operations
- Configuration system and development tooling

**Phase 2A Complete**: Redis Queue Backend ✅
- Production-ready Redis Streams implementation with consumer groups
- Factory and Strategy patterns for pluggable queue backends
- Priority queue system with FIFO ordering within priority levels
- Comprehensive error handling with exponential backoff retry
- Dead letter queue (DLQ) handling for failed tasks
- Batch operations for high-throughput scenarios
- Comprehensive CI/CD pipeline with security scanning and automated testing

**Phase 2B Next**: Worker Engine and CLI interface

## 🚀 Quick Demo

```bash
# Clone and setup
git clone https://github.com/2bxtech/taskforge.git
cd taskforge
make dev-setup

# Start Redis (requires Docker)
docker run -d -p 6379:6379 redis:7-alpine

# Run the Redis queue demo
make demo
```

## 🎯 Goals

Building a production-quality distributed task queue to explore:
- Go concurrency patterns
- Redis Streams for queuing
- Circuit breaker implementations  
- Observability with Prometheus/OpenTelemetry
- Kubernetes deployment patterns

## 🔧 Development Setup

```bash
git clone https://github.com/2bxtech/taskforge.git
cd taskforge
go mod tidy
make build  # Builds current foundation
```

## 📁 Structure

- `pkg/types/` - Core types and interfaces
- `cmd/` - Application entry points (planned)
- `internal/` - Implementation packages (Phase 2)
- `deployments/` - Docker and K8s configs

## 🎯 Learning Focus

- Distributed systems design
- Go best practices and patterns  
- Modern observability stack
- Container orchestration
- API design and documentation

---
*Side project exploring distributed systems concepts with Go*
