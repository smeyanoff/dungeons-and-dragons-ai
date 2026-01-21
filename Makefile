SHELL := /bin/bash

# Prefer docker compose (v2); fallback to docker-compose (v1).
DOCKER_COMPOSE := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

COMPOSE_DEV  := build/docker-compose.yml
COMPOSE_PROD := build/docker-compose.prod.yml
COMPOSE_SEC  := build/docker-compose.security.yml

GO_TEST_TIMEOUT ?= 60m
LLM_TEST_MIN_DELAY_MS ?= 2500

.PHONY: help
help:
	@echo "Common targets:"
	@echo "  docker-up            Start Postgres+Qdrant for dev/tests"
	@echo "  docker-down          Stop dev/test containers"
	@echo "  docker-ps            Show dev/test container status"
	@echo "  test                 Run unit tests (go test ./...)"
	@echo "  test-integration     Run all integration tests (requires containers)"
	@echo "  test-telegram-stub   Telegram gameplay (stable, no real LLM/RAG network calls)"
	@echo "  test-telegram-real   Telegram gameplay with real LLM (GigaChat) (may SKIP if creds missing)"
	@echo "  test-telegram        Telegram gameplay tests (stub + real)"
	@echo ""
	@echo "Production targets:"
	@echo "  deploy               Deploy via build/docker-compose.prod.yml (uses scripts/deploy.sh)"
	@echo "  prod-up/prod-down    Start/stop production compose"
	@echo "  prod-ps/prod-logs    Inspect production containers"
	@echo "  backup/restore       Backup/restore (see DEPLOY.md)"
	@echo ""
	@echo "Security:"
	@echo "  gosec                Run gosec scanner in a container (writes gosec-report.json)"
	@echo "  trivy                Run Trivy FS scan (writes trivy-report.json)"
	@echo "  semgrep              Run Semgrep SAST scan (writes semgrep-report.json)"
	@echo "  snyk                 Run Snyk dependency scan (writes snyk-report.json)"
	@echo "  security-scan        Run all security scans (gosec + Trivy + Semgrep + Snyk)"

.PHONY: docker-up docker-down docker-ps docker-logs docker-down-v
docker-up:
	$(DOCKER_COMPOSE) -f $(COMPOSE_DEV) up -d postgres qdrant

docker-down:
	$(DOCKER_COMPOSE) -f $(COMPOSE_DEV) down

docker-down-v:
	$(DOCKER_COMPOSE) -f $(COMPOSE_DEV) down -v

docker-ps:
	$(DOCKER_COMPOSE) -f $(COMPOSE_DEV) ps

docker-logs:
	$(DOCKER_COMPOSE) -f $(COMPOSE_DEV) logs -f --tail=200

.PHONY: test test-integration
test:
	go test ./...

test-integration:
	LLM_TEST_MIN_DELAY_MS=$(LLM_TEST_MIN_DELAY_MS) go test -v -count=1 -timeout $(GO_TEST_TIMEOUT) ./tests/integration/...

.PHONY: test-telegram-stub test-telegram-real test-telegram
test-telegram-stub:
	go test -v -count=1 -timeout $(GO_TEST_TIMEOUT) ./tests/integration/... -run 'TestTelegramGameplay_BotSimulation_'

test-telegram-real:
	LLM_TEST_MIN_DELAY_MS=$(LLM_TEST_MIN_DELAY_MS) go test -v -count=1 -timeout $(GO_TEST_TIMEOUT) ./tests/integration/... -run 'TestTelegramGameplay_(CompleteFlow|CombatFlow|RealLLM_)'

test-telegram: test-telegram-stub test-telegram-real

.PHONY: test-integration-gameplay
test-integration-gameplay: test-telegram

.PHONY: test-integration-gameplay-docker
test-integration-gameplay-docker:
	@echo "Running gameplay integration tests inside production container (dnd-bot-prod)."
	@echo "NOTE: container must be running and have /root workspace with tests."
	docker exec -it dnd-bot-prod sh -c "cd /root && LLM_TEST_MIN_DELAY_MS=$(LLM_TEST_MIN_DELAY_MS) go test -v -count=1 -timeout $(GO_TEST_TIMEOUT) ./tests/integration/... -run 'TestTelegramGameplay'"

.PHONY: deploy prod-up prod-down prod-build prod-restart prod-ps prod-logs
deploy:
	./scripts/deploy.sh

prod-up:
	$(DOCKER_COMPOSE) -f $(COMPOSE_PROD) up -d

prod-down:
	$(DOCKER_COMPOSE) -f $(COMPOSE_PROD) down

prod-build:
	$(DOCKER_COMPOSE) -f $(COMPOSE_PROD) build

prod-restart:
	$(DOCKER_COMPOSE) -f $(COMPOSE_PROD) restart

prod-ps:
	$(DOCKER_COMPOSE) -f $(COMPOSE_PROD) ps

prod-logs:
	$(DOCKER_COMPOSE) -f $(COMPOSE_PROD) logs -f --tail=200 bot

.PHONY: backup restore monitor-logs
backup:
	./scripts/backup.sh

restore:
	./scripts/restore.sh $(POSTGRES_BACKUP) $(QDRANT_BACKUP)

monitor-logs:
	./scripts/monitor_logs.sh

.PHONY: gosec
gosec:
	$(DOCKER_COMPOSE) -f $(COMPOSE_SEC) run --rm gosec

.PHONY: trivy
trivy:
	$(DOCKER_COMPOSE) -f $(COMPOSE_SEC) run --rm trivy

.PHONY: semgrep
semgrep:
	$(DOCKER_COMPOSE) -f $(COMPOSE_SEC) run --rm semgrep

.PHONY: snyk
snyk:
	$(DOCKER_COMPOSE) -f $(COMPOSE_SEC) --profile with-snyk-token run --rm snyk

.PHONY: security-scan
security-scan: gosec trivy semgrep snyk
