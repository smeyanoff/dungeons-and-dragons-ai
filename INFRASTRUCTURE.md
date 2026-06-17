# Production Infrastructure

Этот документ описывает созданную production-ready инфраструктуру для D&D AI бота.

## Созданные файлы и их назначение

### Docker конфигурация

- **`docker-compose.prod.yml`** - Production конфигурация Docker Compose с:
  - Resource limits для всех сервисов
  - Health checks
  - Security settings (no-new-privileges, read-only FS)
  - Оптимизированное логирование
  - Graceful shutdown

- **`Dockerfile`** (обновлен) - Multi-stage build с:
  - Минимальным финальным образом (alpine)
  - Оптимизациями сборки
  - CA сертификатами для HTTPS

- **`.dockerignore`** - Исключения для Docker build

### Kubernetes манифесты

- **`k8s/namespace.yaml`** - Namespace для приложения
- **`k8s/postgres.yaml`** - PostgreSQL StatefulSet с:
  - PersistentVolumeClaim
  - Secrets
  - Health checks
  - Resource limits

- **`k8s/qdrant.yaml`** - Qdrant Deployment с:
  - PersistentVolumeClaim
  - Health checks
  - Resource limits

- **`k8s/bot.yaml`** - Bot Deployment с:
  - Secrets для чувствительных данных
  - Health checks (liveness/readiness)
  - Resource limits
  - Rolling update strategy

### CI/CD

- **`.github/workflows/ci.yml`** - CI pipeline с:
  - Автоматическими тестами
  - Линтингом
  - Сборкой Docker образов с версионированием
  - Security scanning (Trivy)

- **`.github/workflows/deploy.yml`** - CD pipeline для:
  - Автоматического деплоя при push в main
  - Сборки и публикации образов с версионированием
  - Готовность к интеграции с сервером

- **`.github/workflows/release.yml`** - Release pipeline для:
  - Создания GitHub Release при создании тега
  - Автоматической генерации changelog
  - Публикации Docker образов с версиями

### Скрипты

- **`scripts/deploy.sh`** - Автоматический деплой:
  - Проверка переменных окружения
  - Создание директорий
  - Сборка и запуск сервисов
  - Проверка здоровья сервисов

- **`scripts/backup.sh`** - Создание бэкапов:
  - PostgreSQL dump
  - Qdrant data backup
  - Автоматическая очистка старых бэкапов (7 дней)

- **`scripts/restore.sh`** - Восстановление из бэкапа:
  - Восстановление PostgreSQL
  - Восстановление Qdrant
  - Безопасная остановка сервисов

### Документация

- **`DEPLOY.md`** - Подробное руководство по деплою:
  - Docker Compose деплой
  - Kubernetes деплой
  - Бэкапы и восстановление
  - Мониторинг
  - Troubleshooting

- **`QUICKSTART.md`** - Быстрый старт для production

- **`README.md`** (обновлен) - Добавлена секция про production

### Makefile

Обновлен с командами для:
- Development окружения
- Production окружения
- Деплоя и бэкапов
- Помощи

## Архитектура

### Компоненты

1. **PostgreSQL** - Реляционная БД для:
   - Игровых сессий
   - Персонажей
   - Событий
   - Инвентаря

2. **Qdrant** - Векторное хранилище для:
   - RAG (Retrieval-Augmented Generation)
   - Embeddings документов
   - Семантический поиск

3. **Bot** - Telegram бот приложение с:
   - HTTP health check endpoint
   - Graceful shutdown
   - Обработка игровых действий

### Безопасность

- Secrets в Kubernetes
- Read-only файловая система для контейнеров
- No-new-privileges
- Resource limits
- Health checks

### Версионирование

- Semantic Versioning (SemVer)
- Версия внедряется в приложение через build flags
- Версия доступна через HTTP endpoint `/version`
- Автоматическое версионирование Docker образов
- GitHub Release при создании тегов

Подробности см. [VERSIONING.md](VERSIONING.md)

### Мониторинг

- Health check endpoints (`/health`, `/healthz`, `/readyz`, `/version`)
- Structured logging
- Log rotation
- Готовность к интеграции с Prometheus/Grafana
- LLM Monitoring UI (`http://localhost:8081`) с логами запросов/ответов
- Встроенный LLM proxy (MonitoredLLM) между Telegram и моделью, сохраняет запросы/ответы в PostgreSQL

### Масштабирование

- Горизонтальное масштабирование в Kubernetes
- Resource limits для контроля ресурсов
- Rolling updates для zero-downtime деплоя

## Использование

### Быстрый старт

```bash
make deploy
```

### Управление

```bash
make prod-ps      # Статус
make prod-logs    # Логи
make prod-restart # Перезапуск
make backup       # Бэкап
```

### Kubernetes

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/qdrant.yaml
kubectl apply -f k8s/bot.yaml
```

## Следующие шаги

1. Настройте CI/CD секреты в GitHub
2. Настройте мониторинг (Prometheus/Grafana)
3. Настройте алерты
4. Настройте автоматические бэкапы
5. Настройте SSL/TLS если требуется
6. Настройте CDN для статики (если будет)

## Поддержка

При возникновении проблем см.:
- [DEPLOY.md](DEPLOY.md) - Подробное руководство
- [QUICKSTART.md](QUICKSTART.md) - Быстрый старт
- Логи: `make prod-logs`
- Health checks: `curl http://localhost:8080/health`
