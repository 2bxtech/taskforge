# TaskForge status and scope

## Purpose

TaskForge is a learning prototype for Go queue and worker primitives. Its supported artifact is the library code under `internal/queue` and `internal/worker`, exercised through `examples/` and tests. It is not presented as an operational distributed task-queue product.

## Component status

| Component | Status | Evidence and limits |
|---|---|---|
| Redis queue backend | Implemented prototype | Redis Streams, consumer groups, task hashes, acknowledgement, retry scheduling, and DLQ operations |
| Worker engine | Implemented prototype | Polling, command dispatch, observer events, graceful-stop machinery, and failure handling |
| Retry delivery | Implemented prototype | Failed deliveries are acknowledged, persisted, scheduled in a sorted set, and promoted when a worker polls the target queue |
| Dead-letter handling | Implemented prototype | Exhausted tasks are acknowledged and copied to a queue-specific DLQ stream |
| Bulkhead/circuit breaker/rate limiter | Implemented patterns | In-process controls only; resource quantities are admission budgets rather than OS enforcement |
| Examples | Runnable | Require a reachable Redis instance |
| API, CLI, worker service, scheduler service | Scaffolding | `cmd/` programs intentionally do not claim operational behavior |
| PostgreSQL/NATS backends and production telemetry | Not implemented | Types/configuration may reserve future design space |

## Delivery semantics

1. `Enqueue` writes the serialized task to a task hash and appends a Redis Stream entry.
2. `Dequeue` first promotes due retries for the requested queue, then reads through a consumer group.
3. A successful worker execution calls `Ack`, acknowledges the Stream delivery, and marks the task completed.
4. A failed execution with attempts remaining persists the incremented retry state, acknowledges the current delivery, and adds the task ID to the scheduled sorted set.
5. A later queue poll atomically removes a due scheduled entry and appends its replacement delivery using a Redis script.
6. A failure with no attempts remaining acknowledges the delivery and moves the task to the queue-specific DLQ stream.

The retry promotion step is deliberately worker-driven. If no worker polls a queue, retries for that queue remain scheduled.
The scheduled-set key is configurable so independent TaskForge instances can isolate their retry state.

## Resource-budget semantics

`BulkheadConfig.MaxTotalMemoryMB` is an explicit logical budget with a 512 MB default. It is not derived from `runtime.MemStats.Sys`: that metric reports memory obtained by the Go runtime, not total or available host memory. Operators embedding the library should configure the budget for their workload.

## Verification

Run:

```bash
go test ./...
go test -race ./...
go test -tags=integration ./tests/integration
```

The integration suite expects Redis at `localhost:6379` and verifies the delivery lifecycle through a real worker processor.
