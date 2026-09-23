SHELL := /bin/sh
GO := $(if $(wildcard .tools/go/bin/go),$(CURDIR)/.tools/go/bin/go,go)
GOFMT := $(if $(wildcard .tools/go/bin/gofmt),$(CURDIR)/.tools/go/bin/gofmt,gofmt)
SQLC := $(if $(wildcard .tools/bin/sqlc),$(CURDIR)/.tools/bin/sqlc,sqlc)
export PATH := $(CURDIR)/.tools/bin:$(CURDIR)/.tools/go/bin:$(PATH)
DATABASE_URL ?= postgres://apm:apm@localhost:55432/apm?sslmode=disable
export DATABASE_URL

.PHONY: help deps generate fmt lint test test-integration verify db-up db-down migrate build demo demo-live eval eval-live feedback-demo
help:
	@printf '%s\n' 'make demo: start PostgreSQL, migrate, build, replay queue-delay (no API key)' 'make feedback-demo: demonstrate feedback, reviewed memory and disabling (mock)' 'make verify: generated code, formatting, vet, unit/integration tests, mock eval' 'make demo-live / eval-live: real OpenAI API; requires OPENAI_API_KEY' 'make db-down: stop PostgreSQL; retain data volume'
deps:
	@$(GO) mod download
	@GOBIN=$(CURDIR)/.tools/bin $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
generate:
	@$(SQLC) generate
fmt:
	@$(GOFMT) -w cmd internal db/migrate.go
lint:
	@$(GO) vet ./...
test:
	@$(GO) test ./...
	@echo 'UNIT TESTS PASSED'
test-integration: db-up migrate
	@APM_INTEGRATION=1 $(GO) test -count=1 -run Integration ./internal/integration
	@echo 'INTEGRATION TESTS PASSED'
verify:
	@python3 scripts/check-generated.py
	@test -z "$$("$(GOFMT)" -l cmd internal db/migrate.go)" || (echo 'Run make fmt'; exit 1)
	@$(MAKE) --no-print-directory lint test test-integration eval
	@python3 scripts/cli-smoke.py
	@python3 scripts/feedback-demo.py
	@echo 'VERIFICATION PASSED'
db-up:
	@docker compose --progress quiet up -d --wait
db-down:
	@docker compose down
migrate:
	@$(GO) run ./cmd/apm migrate > /dev/null
build:
	@$(GO) build -o bin/apm ./cmd/apm
demo: db-up migrate build
	@./bin/apm replay queue-delay --ai-mode mock
demo-live:
	@test -n "$$OPENAI_API_KEY" || (echo 'live mode requires OPENAI_API_KEY'; exit 1)
	@$(MAKE) --no-print-directory db-up migrate build
	@./bin/apm replay queue-delay --ai-mode live
eval: db-up migrate build
	@./bin/apm eval --ai-mode mock
eval-live:
	@test -n "$$OPENAI_API_KEY" || (echo 'live mode requires OPENAI_API_KEY'; exit 1)
	@$(MAKE) --no-print-directory db-up migrate build
	@./bin/apm eval --ai-mode live

feedback-demo: db-up migrate build
	@python3 scripts/feedback-demo.py
