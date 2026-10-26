# 0009. SQLite over PostgreSQL

Status: Accepted

## Context
The brief asks for "a simple, easy to use database backend appropriate for the task" and for all processes to "easily run inside the same container for trivial deployment." PostgreSQL is more robust/scalable but requires a separate DB process/container. SQLite is embedded and file-based.

## Decision
Use embedded SQLite (e.g. `modernc.org/sqlite`) as the database, a single file inside the same container as the app binary.

## Consequences
- No separate DB container/process to deploy, configure, or coordinate startup with — fits the "trivial deployment" constraint directly.
- Write concurrency is more limited than Postgres; acceptable because this is a turn-based, low-throughput, casual app, not a high-concurrency service.
- If scale requirements change materially later, this decision should be revisited explicitly (new ADR), not silently swapped.
