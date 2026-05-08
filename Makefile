BINARY_NAME    := claude-ops
OPSCTL_NAME    := claude-opsctl
CMD_DIR        := cmd/claude-ops
OPSCTL_DIR     := cmd/claude-opsctl
BIN_DIR        := bin
COVERAGE_FILE  := coverage.out
MIGRATIONS_DIR := migrations
# MYSQL_DSN must include parseTime=true&charset=utf8mb4. Override on the
# command line: make migrate-up MYSQL_DSN='user:pass@tcp(host)/db?...'.
MYSQL_DSN      ?= claude_ops:claude_ops@tcp(127.0.0.1:3306)/claude_ops?parseTime=true&charset=utf8mb4&loc=UTC
CONFIG_PATH    ?= config.example.yaml

.PHONY: build run test cover lint swag sqlc migrate-up migrate-down docker clean help \
        opsctl-build opsctl-release

## build: compile single binary (pure-Go, no CGO required for MySQL driver)
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)/...

## run: run the agent with example config
run: build
	./$(BIN_DIR)/$(BINARY_NAME) -config $(CONFIG_PATH)

## test: run all tests with race detector
test:
	go test -race -count=1 ./...

## cover: run tests and show coverage
cover:
	go test -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	go tool cover -func=$(COVERAGE_FILE) | tail -5

## cover-html: open coverage in browser
cover-html: cover
	go tool cover -html=$(COVERAGE_FILE)

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## swag: regenerate swagger docs
swag:
	swag init -g $(CMD_DIR)/main.go -o docs/swagger

## sqlc: generate sqlc code
sqlc:
	sqlc generate

## migrate-up: run all pending migrations against MYSQL_DSN
migrate-up:
	migrate -source file://$(MIGRATIONS_DIR) -database "mysql://$(MYSQL_DSN)" up

## migrate-down: rollback last migration
migrate-down:
	migrate -source file://$(MIGRATIONS_DIR) -database "mysql://$(MYSQL_DSN)" down 1

## docker: build docker image
docker:
	docker build -f deployments/Dockerfile -t $(BINARY_NAME):latest .

## opsctl-build: compile claude-opsctl for current platform
opsctl-build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN_DIR)/$(OPSCTL_NAME) ./$(OPSCTL_DIR)/...

## opsctl-release: cross-compile claude-opsctl for linux/amd64, linux/arm64, darwin/arm64
opsctl-release:
	@mkdir -p $(BIN_DIR)
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" \
	  -o $(BIN_DIR)/$(OPSCTL_NAME)-linux-amd64 ./$(OPSCTL_DIR)/...
	GOOS=linux  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" \
	  -o $(BIN_DIR)/$(OPSCTL_NAME)-linux-arm64 ./$(OPSCTL_DIR)/...
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" \
	  -o $(BIN_DIR)/$(OPSCTL_NAME)-darwin-arm64 ./$(OPSCTL_DIR)/...

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR) $(COVERAGE_FILE)

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
