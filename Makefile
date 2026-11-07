.PHONY: help build run test test-short test-unit test-browser test-private test-sims smoke lint fmt vet cover clean

help: ## list the targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/\t/'

build: ## build bin/bingle
	go build -o bin/bingle .

run: build ## build and run on http://localhost:8080
	./bin/bingle

test: ## the whole suite and the smoke test
	go test -timeout 25m ./...
	@$(MAKE) --no-print-directory smoke

test-short: ## skip the slowest tests
	go test -short ./...

test-unit: ## everything but the browser suite and the bot simulations (about a minute)
	go test -timeout 10m $$(go list ./... | grep -vE '/tests/(app|batch)$$')

test-browser: ## the browser suite; SHARD=i/n runs one shard of it, as CI does
	BINGLE_TEST_SHARD=$(SHARD) go test -timeout 15m ./tests/app/

# The private overlay (ADR 0127) is a checkout at internal/games/private. Its
# browser tests share the public helpers of tests/app, so they are copied in
# (as private_*_test.go, which git ignores) while the suite runs, and taken out
# again after, whatever the result.
test-private: ## the whole suite with the private overlay's games (-tags private)
	@test -d internal/games/private || { echo "no private overlay checked out at internal/games/private"; exit 1; }
	@for f in internal/games/private/tests/_app/*_test.go; do cp "$$f" "tests/app/private_$$(basename "$$f")"; done
	@go test -tags private -timeout 40m ./...; status=$$?; for f in tests/app/private_*_test.go; do unlink "$$f"; done; exit $$status

test-sims: ## the long bot simulations (about six minutes on two cores)
	go test -timeout 20m ./tests/batch/

smoke: ## run the real binary as a real process and exercise it over HTTP
	./scripts/smoke-test.sh

fmt: ## format the code
	gofmt -w .

vet: ## go vet
	go vet ./...

lint: ## gofmt check and golangci-lint (if installed)
	@echo "gofmt check:"
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else echo "golangci-lint not installed locally; skipping (CI runs it)"; fi

cover: ## a coverage report of the engine and the server (cover.html)
	go test -coverpkg=./internal/... -coverprofile=cover.out ./tests/...
	go tool cover -html=cover.out -o cover.html

clean: ## remove build output
	rm -rf bin cover.out cover.html
