# 🔨 TaskForge

A distributed task queue system side project built with Go. Exploring modern distributed systems patterns and potentially developing into a portfolio piece.

## 🏗️ Current Status

**Phase 1 Complete**: Core architecture and type system
- Complete type definitions for task management
- Interface design for queue backends and workers
- 6 task types: webhooks, email, image processing, data processing, scheduled tasks, batch operations
- Configuration system and development tooling

**Phase 2 Planned**: Redis implementation and working processors

## 🚀 Goals

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