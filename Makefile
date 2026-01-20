.PHONY: fmt build test test-telegram test-integration docker-build docker-up docker-down docker-logs docker-restart \
	prod-build prod-up prod-down prod-logs prod-restart prod-ps \
	deploy backup restore help security-scan scan-docker scan-code scan-dockerfile \
	full-scan

# Загружаем переменные окружения из .env файла
ifneq (,$(wildcard .env))
    include .env
    export
endif

# Форматирование кода
fmt:
	go fmt ./...

# Переменные для версии
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Сборка приложения
build:
	go build -ldflags="-X dungeons-and-dragons-ai/pkg/version.Version=$(VERSION) -X dungeons-and-dragons-ai/pkg/version.Commit=$(COMMIT) -X dungeons-and-dragons-ai/pkg/version.BuildTime=$(BUILD_TIME)" -o bin/bot ./cmd/bot

# Запуск всех тестов (загружает .env автоматически)
test:
	@echo "Запуск всех тестов с использованием .env..."
	go test -v -race -coverprofile=coverage.out ./... -timeout 30m

# Запуск Telegram gameplay тестов (загружает .env автоматически)
test-telegram:
	@echo "Запуск Telegram gameplay тестов с использованием .env..."
	go test -v ./tests/integration/... -run TestTelegramGameplay -timeout 30m

# Запуск всех интеграционных тестов (загружает .env автоматически)
test-integration:
	@echo "Запуск всех интеграционных тестов с использованием .env..."
	go test -v ./tests/integration/... -timeout 30m

# Запуск всех интеграционных тестов внутри Docker контейнера
test-integration-docker:
	@echo "Запуск всех интеграционных тестов внутри Docker контейнера..."
	@echo "Убедитесь, что контейнеры запущены: make docker-up"
	@echo "В контейнере сертификаты Сбербанка уже настроены, GIGACHAT_SKIP_TLS_VERIFY не требуется"
	@echo "Результаты будут записаны в TESTING_REPORT.md и FEEDBACK.md"
	@docker exec dnd-bot-prod sh -c "cd /root && go test -v -timeout 60m ./tests/integration/..."

# ============================================
# Development (docker-compose.yml)
# ============================================

# Сборка Docker образа для разработки
docker-build:
	docker-compose -f build/docker-compose.yml build

# Запуск всех сервисов для разработки
docker-up:
	docker-compose -f build/docker-compose.yml up -d

# Остановка всех сервисов
docker-down:
	docker-compose -f build/docker-compose.yml down

# Остановка с удалением volumes
docker-down-v:
	docker-compose -f build/docker-compose.yml down -v

# Просмотр логов
docker-logs:
	docker-compose -f build/docker-compose.yml logs -f bot

# Перезапуск бота
docker-restart:
	docker-compose -f build/docker-compose.yml restart bot

# Просмотр статуса сервисов
docker-ps:
	docker-compose -f build/docker-compose.yml ps

# ============================================
# Production (docker-compose.prod.yml)
# ============================================

# Сборка Docker образа для production
prod-build:
	docker-compose -f build/docker-compose.prod.yml build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME)

# Запуск всех сервисов в production
prod-up:
	docker-compose -f build/docker-compose.prod.yml up -d

# Остановка всех сервисов в production
prod-down:
	docker-compose -f build/docker-compose.prod.yml down

# Просмотр логов production
prod-logs:
	docker-compose -f build/docker-compose.prod.yml logs -f bot

# Перезапуск бота в production
prod-restart:
	docker-compose -f build/docker-compose.prod.yml restart bot

# Просмотр статуса сервисов в production
prod-ps:
	docker-compose -f build/docker-compose.prod.yml ps

# Полный деплой в production
deploy:
	@./scripts/deploy.sh

# Создание бэкапа
backup:
	@./scripts/backup.sh

# Восстановление из бэкапа
restore:
	@echo "Использование: make restore POSTGRES_BACKUP=backups/postgres_xxx.sql.gz QDRANT_BACKUP=backups/qdrant_xxx.tar.gz"
	@./scripts/restore.sh $(POSTGRES_BACKUP) $(QDRANT_BACKUP)

# Помощь
help:
	@echo "Доступные команды:"
	@echo ""
	@echo "Разработка:"
	@echo "  make docker-build    - Сборка Docker образа"
	@echo "  make docker-up      - Запуск сервисов"
	@echo "  make docker-down    - Остановка сервисов"
	@echo "  make docker-logs    - Просмотр логов"
	@echo "  make docker-restart - Перезапуск бота"
	@echo ""
	@echo "Production:"
	@echo "  make prod-build      - Сборка production образа"
	@echo "  make prod-up         - Запуск production сервисов"
	@echo "  make prod-down       - Остановка production сервисов"
	@echo "  make prod-logs       - Просмотр production логов"
	@echo "  make deploy          - Полный деплой в production"
	@echo "  make backup          - Создание бэкапа"
	@echo "  make restore         - Восстановление из бэкапа"
	@echo ""
	@echo "Утилиты:"
	@echo "  make fmt             - Форматирование кода"
	@echo "  make build           - Сборка приложения"
	@echo "  make test            - Запуск всех тестов (использует .env)"
	@echo "  make test-telegram   - Запуск Telegram gameplay тестов (использует .env)"
	@echo "  make test-integration - Запуск интеграционных тестов (использует .env)"
	@echo "  make test-integration-docker - Запуск тестов внутри контейнера"
	@echo ""
	@echo "Безопасность:"
	@echo "  make security-scan   - Полное сканирование безопасности"
	@echo "  make scan-docker     - Сканирование Docker образов (Trivy)"
	@echo "  make scan-code       - Сканирование Go кода (Gosec)"
	@echo "  make scan-dockerfile - Сканирование Dockerfile (Hadolint)"

# ============================================
# Security Scanning
# ============================================

# Полное сканирование безопасности
security-scan: scan-dockerfile scan-code scan-docker
	@echo "✓ Полное сканирование безопасности завершено"

# Сканирование Docker образов с помощью Trivy
scan-docker:
	@echo "🔍 Сканирование Docker образов..."
	@if command -v trivy >/dev/null 2>&1; then \
		echo "Сканирование образа dnd-bot:latest..."; \
		trivy image --severity HIGH,CRITICAL --format table dnd-bot:latest || true; \
		trivy image --severity HIGH,CRITICAL --format json -o trivy-image-report.json dnd-bot:latest || true; \
		echo "Сканирование образа postgres:16-alpine..."; \
		trivy image --severity HIGH,CRITICAL --format table postgres:16-alpine || true; \
		echo "Сканирование образа qdrant/qdrant:v1.16.2..."; \
		trivy image --severity HIGH,CRITICAL --format table qdrant/qdrant:v1.16.2 || true; \
	elif command -v docker >/dev/null 2>&1; then \
		echo "Используем Trivy через Docker контейнер..."; \
		docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v $(PWD):/workspace -w /workspace aquasec/trivy:latest image --severity HIGH,CRITICAL --format table dnd-bot:latest || true; \
		docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v $(PWD):/workspace -w /workspace aquasec/trivy:latest image --severity HIGH,CRITICAL --format json -o trivy-image-report.json dnd-bot:latest || true; \
	else \
		echo "⚠️  Trivy не установлен и Docker недоступен. Установите: brew install trivy"; \
	fi

# Сканирование Go кода с помощью Gosec
scan-code:
	@echo "🔍 Сканирование Go кода на уязвимости..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec -fmt json -out gosec-report.json ./... || true; \
		gosec -fmt text ./... || true; \
	elif command -v docker >/dev/null 2>&1; then \
		echo "Используем Gosec через Docker контейнер..."; \
		docker run --rm -v $(PWD):/app -w /app securego/gosec:latest -fmt json -out gosec-report.json ./... || true; \
		docker run --rm -v $(PWD):/app -w /app securego/gosec:latest -fmt text ./... || true; \
	else \
		echo "⚠️  Gosec не установлен и Docker недоступен. Установите: go install github.com/golangci/gosec/v2/cmd/gosec@latest"; \
	fi

# Сканирование Dockerfile с помощью Hadolint
scan-dockerfile:
	@echo "🔍 Сканирование Dockerfile на лучшие практики..."
	@if command -v hadolint >/dev/null 2>&1; then \
		hadolint build/Dockerfile || true; \
		hadolint --format json build/Dockerfile > hadolint-report.json 2>&1 || true; \
	elif docker run --rm -i hadolint/hadolint < build/Dockerfile; then \
		echo "✓ Dockerfile проверен через Docker контейнер Hadolint"; \
	else \
		echo "⚠️  Hadolint не установлен. Установите: brew install hadolint или используйте Docker: docker run --rm -i hadolint/hadolint < build/Dockerfile"; \
	fi
