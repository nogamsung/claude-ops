BINARY_NAME    := scheduled-dev-agent
CMD_DIR        := cmd/scheduled-dev-agent
BIN_DIR        := bin
COVERAGE_FILE  := coverage.out
MIGRATIONS_DIR := migrations
DB_PATH        ?= data/agent.db
CONFIG_PATH    ?= config.example.yaml

.PHONY: build run test cover lint swag sqlc migrate-up migrate-down docker clean help

## build: compile single binary
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)/...

## run: run the agent with example config
run: build
	./$(BIN_DIR)/$(BINARY_NAME) -config $(CONFIG_PATH)

## test: run all tests with race detector
test:
	CGO_ENABLED=1 go test -race -count=1 ./...

## cover: run tests and show coverage
cover:
	CGO_ENABLED=1 go test -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
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

## migrate-up: run all pending migrations
migrate-up:
	migrate -source file://$(MIGRATIONS_DIR) -database "sqlite3://$(DB_PATH)" up

## migrate-down: rollback last migration
migrate-down:
	migrate -source file://$(MIGRATIONS_DIR) -database "sqlite3://$(DB_PATH)" down 1

## docker: build docker image
docker:
	docker build -f deployments/Dockerfile -t $(BINARY_NAME):latest .

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR) $(COVERAGE_FILE)

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
