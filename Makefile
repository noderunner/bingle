.PHONY: help build run test test-short test-unit test-browser test-cw-ui test-sims smoke lint fmt vet cover clean

help: ## list the targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/\t/'

build: ## build bin/bingle
	go build -o bin/bingle .

run: build ## build and run on http://localhost:8080
	./bin/bingle

test: ## the whole suite, including the Cthulhu Wars browser tests and the smoke test
	BINGLE_TEST_CW_UI=1 go test -timeout 25m ./...
	@$(MAKE) --no-print-directory smoke

test-short: ## skip the slowest tests
	go test -short ./...

test-unit: ## everything but the browser suite and the bot simulations (about a minute)
	go test -timeout 10m $$(go list ./... | grep -vE '/tests/(app|batch)$$')

test-browser: ## the browser suite; SHARD=i/n runs one shard of it, as CI does
	BINGLE_TEST_SHARD=$(SHARD) go test -timeout 15m ./tests/app/

test-cw-ui: ## the Cthulhu Wars browser tests, which are off by default
	BINGLE_TEST_CW_UI=1 go test -timeout 20m ./tests/app/ -run TestWebCthulhuWars

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
