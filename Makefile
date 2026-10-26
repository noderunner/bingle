.PHONY: help build run test test-short lint fmt vet cover clean

help: ## list the targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/\t/'

build: ## build bin/bingle
	go build -o bin/bingle .

run: build ## build and run on http://localhost:8080
	./bin/bingle

test: ## the whole suite (about 6 minutes, with the browser tests)
	go test ./...

test-short: ## skip the slowest tests
	go test -short ./...

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
