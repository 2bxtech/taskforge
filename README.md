# TaskForge

[![CI](https://img.shields.io/github/actions/workflow/status/2bxtech/taskforge/ci.yml?branch=main&label=build)](https://github.com/2bxtech/taskforge/actions/workflows/ci.yml)
[![Integration](https://img.shields.io/github/actions/workflow/status/2bxtech/taskforge/integration.yml?branch=main&label=integration)](https://github.com/2bxtech/taskforge/actions/workflows/integration.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

TaskForge is a bounded Go prototype for exploring task-queue and worker fault-tolerance patterns. It is a library and demonstration project, not a production-ready queue service.

## Implemented

- Redis Streams queue backend with consumer groups
- Enqueue, dequeue, acknowledgement, retry scheduling, and dead-letter handling
- Worker engine using command and observer patterns
- Bulkhead-style admission control, circuit breakers, and rate limiters
- Redis and worker-engine demonstrations
- Unit tests plus a Redis-backed delivery-path integration test

## Explicitly out of scope

- The programs under `cmd/` are labeled scaffolding; they are not deployable API, CLI, worker, or scheduler services.
- Scheduled retries are promoted opportunistically by workers polling a queue. There is no independent scheduler service.
- Resource limits are logical admission-control budgets. They do not enforce operating-system CPU or memory limits.
- Queue statistics and observability are prototype-level, not production telemetry.
- PostgreSQL, NATS, authentication, and the broader configuration surface are design placeholders.

See [docs/status-and-scope.md](docs/status-and-scope.md) for the component inventory and delivery semantics.

## Run the demonstrations

Requirements: Go 1.23+, Docker, and `make`.

```bash
docker run -d --name taskforge-redis -p 6379:6379 redis:7-alpine
make redis-demo
make worker-demo
```

The worker demo runs until interrupted.

## Test

```bash
go test ./...

# Requires Redis on localhost:6379
go test -tags=integration ./tests/integration
```

The integration test covers enqueue → worker execution → scheduled retry → second execution → dead-letter state.

## Project layout

```text
pkg/types/              Public interfaces and task/configuration types
internal/queue/redis/   Redis Streams queue implementation
internal/worker/        Worker engine and fault-tolerance patterns
examples/               Runnable demonstrations
tests/integration/      Redis-backed delivery-path test and demo smoke scripts
cmd/                    Non-functional application scaffolding
docs/                   Scope and assessment records
```

## Build

```bash
go build ./...
```

This compilation check includes the scaffolding under `cmd/`; a successful build does not imply that those programs provide services.
