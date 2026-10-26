# 0013. net/http + lightweight frontend, no SPA build

Status: Accepted

## Context
Considered three stacks: Go stdlib `net/http` + chi/gorilla router with a lightweight JS/htmx frontend; stdlib backend with a React/TypeScript SPA (a very common, well-supported combination but requiring a JS build pipeline); or gRPC with grpc-web (adds a proxy requirement for browser support).

## Decision
Backend on Go stdlib `net/http` with a chi/gorilla-style router. Frontend is lightweight JS (or htmx), with no SPA build step.

## Consequences
- No Node/Vite build pipeline to run, deploy, or keep in sync with the Go binary — fits the single-binary/single-container deployment goal ([0014](0014-single-binary-process-model.md)).
- Rich interactive board UI (drag/drop, animations) is more manual to build than it would be with a component framework — acceptable since the brief explicitly excludes animation for v1.
- If the UI's interactivity needs grow substantially later, this is worth revisiting deliberately rather than accreting ad hoc JS indefinitely.
