# 0012. TLS optional, off by default

Status: Accepted

## Context
The brief requires TLS support for API connections. Considered: TLS always required (auto-generating a self-signed cert when none is configured), versus TLS optional and off by default for local/dev with easy config-driven enablement for remote deployments.

## Decision
TLS is supported but optional and off by default. It's enabled by supplying cert/key paths in config; local/dev/container testing runs plain HTTP unless configured otherwise.

## Consequences
- Local and CI testing needs no cert generation/management step.
- Anyone deploying remotely must explicitly configure TLS — it is not on by default, so a careless remote deployment could run without it. This is an accepted tradeoff for simplicity, not an oversight.
