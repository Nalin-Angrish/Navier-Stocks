# =============================================================================
# Navier-Stocks — Makefile
# =============================================================================
# Development and operations convenience targets for the multi-agent trading
# system.  Run `make help` to see all available targets.
# =============================================================================

# -- Declare phony targets so make knows they aren't files --------------------
.PHONY: help infra up down run build docker-build test test/short test/integration test/coverage \
        lint tidy vet fmt clean db/migrate db/rollback psql logs

# -- Variables ----------------------------------------------------------------
BIN_NAME  := navier-stocks
BUILD_DIR := ./bin
CMD_DIR   := ./src/main
GO_FILES  := $(shell find . -name '*.go' -not -path './.git/*' -not -path './vendor/*' -not -path './dashboard/*')
GO_PKGS   := $(shell go list ./... | grep -v /vendor/ | grep -v /dashboard/)

# -- Development workflow -----------------------------------------------------

## help:      Print this help message
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' ${MAKEFILE_LIST} | column -t -s ':'

## infra:     Start infrastructure dependencies (NATS + PostgreSQL) in background
infra:
	@docker compose up -d nats postgres

## run:       Build the binary and run it locally
run: build
	@$(BUILD_DIR)/$(BIN_NAME)

# -- Build -------------------------------------------------------------------

## build:     Compile a statically-linked binary into ./bin/
build:
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BIN_NAME) $(CMD_DIR)

## docker-build: Build the application Docker image (cached via docker compose)
docker-build:
	@docker compose build app

# -- Testing -----------------------------------------------------------------

## test:           Run all tests with race detection enabled
test:
	@CGO_ENABLED=1 go test -race -count=1 $(GO_PKGS)

## test/short:     Run tests without race detection (faster feedback)
test/short:
	@go test -count=1 $(GO_PKGS)

## test/integration: Run only integration tests (require live Groww API credentials)
test/integration:
	@CGO_ENABLED=1 go test -tags=integration -race -count=1 -run '^TestLive' $(GO_PKGS)

## test/coverage:  Run tests, generate an HTML coverage report
test/coverage:
	@CGO_ENABLED=1 go test -race -count=1 -coverprofile=coverage.out -covermode=atomic $(GO_PKGS)
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out

# -- Code quality ------------------------------------------------------------

## lint:      Run golangci-lint (auto-installs if missing)
lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@golangci-lint run --timeout 5m

## vet:       Run go vet (static analysis baked into the Go toolchain)
vet:
	@go vet $(GO_PKGS)

## fmt:       Format all Go source files with gofmt
fmt:
	@gofmt -l -w $(GO_FILES)

## tidy:      Tidy Go module dependencies and verify the checksums
tidy:
	@go mod tidy
	@go mod verify

# -- Housekeeping ------------------------------------------------------------

## clean:     Remove build artifacts, coverage reports, and temp files
clean:
	@rm -rf $(BUILD_DIR) coverage.out coverage.html tmp/

# -- Infrastructure orchestration --------------------------------------------

## up:        Build and start the full stack (NATS + PostgreSQL + app)
up:
	@docker compose up -d --build

## down:      Stop and remove all containers (volumes are preserved)
down:
	@docker compose down

## logs:      Tail logs from every running container
logs:
	@docker compose logs -f

# -- Database helpers --------------------------------------------------------
# These assume a local PostgreSQL is reachable via PG_HOST / PG_USER / PG_DATABASE
# environment variables (defaults: localhost / navier / navier_stocks).

## db/migrate:    Apply all pending SQL migration files against the local database
db/migrate:
	@echo "Running migrations from src/migrations..."
	@for f in src/migrations/*.up.sql; do \
		echo "Applying $${f}..."; \
		PGPASSWORD=${PG_PASSWORD} psql -h ${PG_HOST} -U ${PG_USER} -d ${PG_DATABASE} -f "$${f}"; \
	done

## db/rollback:   Roll back the most recent migration
db/rollback:
	@echo "Rolling back..."
	@for f in $$(ls -r src/migrations/*.down.sql); do \
		echo "Applying $${f}..."; \
		PGPASSWORD=${PG_PASSWORD:-devpassword} psql -h ${PG_HOST:-localhost} -U ${PG_USER:-navier} -d ${PG_DATABASE:-navier_stocks} -f "$${f}"; \
		break; \
	done

## psql:      Open an interactive psql shell into the local database
psql:
	@PGPASSWORD=${PG_PASSWORD:-devpassword} psql -h ${PG_HOST:-localhost} -U ${PG_USER:-navier} -d ${PG_DATABASE:-navier_stocks}
