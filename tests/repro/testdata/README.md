# Bundles kept as regression tests

A repro bundle in this directory is replayed by `TestRepro` in the package
above, and **the test fails if it reproduces**. So a bundle belongs here once
the fault it holds has been fixed: it then stands as the regression test for
that fix, in the form the fault actually arrived in.

To look at a bundle that has *not* been fixed yet, don't put it here — point
the test at it, or use the command:

```sh
go test ./tests/repro/ -run 'TestRepro$' -bundle /var/lib/bingle/debug/<bundle>.json
go run ./scripts/replay /var/lib/bingle/debug/<bundle>.json
```

Bundles carry a whole position, including committed dice, hidden hands and the
game's random state (that is what makes them reproduce). A bundle from a real
server is therefore a real game's secrets: check what is in one before
committing it, and prefer one produced from a test or a batch.

See [docs/observability.md](../../../docs/observability.md).
