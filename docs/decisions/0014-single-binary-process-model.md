# 0014. Single binary, goroutines instead of multiple processes

Status: Accepted

## Context
The brief requires that "all individually running processes should easily run inside the same container for trivial deployment and testing." The API, web UI, and MCP server could be genuinely separate OS processes coordinated by a lightweight supervisor (e.g. s6, supervisord) inside the container, or internal services within one Go binary started as goroutines from one `main()`.

## Decision
One Go binary. The HTTP server (REST + WebSocket + static UI) and the MCP server run as goroutines from a single `main()`, sharing one in-process game engine and one SQLite connection pool.

## Consequences
- No process supervisor, no inter-process coordination/health-checking needed — the container entrypoint is just the binary.
- All components share one address space and fail together; there's no isolation between, say, an MCP server bug and the HTTP server. Acceptable for a casual v1 app with no isolation requirement in the brief.
- Scaling or isolating a component later (e.g. running MCP separately) would mean splitting this binary — a deliberate future decision, not something to design around now.
