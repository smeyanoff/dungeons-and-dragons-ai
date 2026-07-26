# Деплой в Production

Руководство по развёртыванию D&D AI бота в production (Docker Compose и Kubernetes).

## Предварительные требования

- Docker и Docker Compose
- Сервер: минимум 4GB RAM, 2 CPU cores, 20GB свободного места
- Настроенные переменные окружения (см. `.env.example`)
- GigaChat API credentials и Telegram Bot Token

## Быстрый старт

```bash
git clone <repository-url>
cd dungeons-and-dragons-ai
cp .env.example .env
# отредактируйте .env: TELEGRAM_BOT_TOKEN, GIGACHAT_CLIENT_ID, GIGACHAT_CLIENT_SECRET, POSTGRES_PASSWORD

make deploy       # build + up через build/docker-compose.prod.yml
make prod-ps      # статус контейнеров
make prod-logs    # логи

curl http://localhost:8080/health   # health check
open http://localhost:8081          # LLM Monitoring UI (см. .claude/skills/read-dnd-bot-logs)
```

`make deploy` — единственный поддерживаемый способ (пере)запуска прод-контейнеров; ручные
`docker compose` команды ниже — для справки/troubleshooting.

## Деплой через Docker Compose (вручную)

```bash
docker compose -f build/docker-compose.prod.yml build
docker compose -f build/docker-compose.prod.yml up -d
```

### Структура production-конфигурации

`build/docker-compose.prod.yml` включает:
- Resource limits для всех сервисов
- Health checks
- Security settings (`no-new-privileges`, read-only файловая система)
- Graceful shutdown

### Переменные окружения

**Обязательные:**
- `TELEGRAM_BOT_TOKEN` — токен Telegram бота
- `GIGACHAT_CLIENT_ID` / `GIGACHAT_CLIENT_SECRET` — credentials GigaChat API
- `POSTGRES_PASSWORD` — пароль для PostgreSQL

**Опциональные:**
- `POSTGRES_USER` (по умолчанию `dnd_user`), `POSTGRES_DB` (по умолчанию `dnd`)
- `GIGACHAT_MODEL` — модель для генерации текста DM (по умолчанию `GigaChat`).
  Доступные: `GigaChat`, `GigaChat-Plus`, `GigaChat-Pro`, `GigaChat-Max`,
  `GigaChat-2`, `GigaChat-2-Pro`, `GigaChat-2-Max` (см. `pkg/gigachat/models.go`)
- `GIGACHAT_ANALYZER_MODEL` — отдельная (обычно более дешёвая) модель для
  структурных LLM-вызовов `dm_analyzer` (бой/квесты/предметы, pre-check проверок
  навыков) — не творческая генерация, топовая модель DM здесь не нужна
  (по умолчанию `GigaChat-2`)
- `GIGACHAT_EMBEDDINGS_MODEL` — модель для эмбеддингов/RAG (по умолчанию `Embeddings`;
  доступна также `EmbeddingsGigaR`)
- `MONITORING_PORT` — порт LLM Monitoring UI (по умолчанию `8081`)
- `LOG_LEVEL` — уровень логирования (по умолчанию `info`)

## Деплой в Kubernetes

### Предварительные требования

- Kubernetes кластер (1.20+), `kubectl`, доступ к Container Registry

### Шаги

```bash
kubectl apply -f k8s/namespace.yaml

kubectl create secret generic postgres-secret \
  --from-literal=POSTGRES_USER=dnd_user \
  --from-literal=POSTGRES_PASSWORD=your_strong_password \
  --from-literal=POSTGRES_DB=dnd \
  -n dnd-ai

kubectl create secret generic bot-secret \
  --from-literal=TELEGRAM_BOT_TOKEN=your_token \
  --from-literal=GIGACHAT_CLIENT_ID=your_client_id \
  --from-literal=GIGACHAT_CLIENT_SECRET=your_client_secret \
  -n dnd-ai

kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/qdrant.yaml
kubectl apply -f k8s/bot.yaml

kubectl get pods -n dnd-ai
kubectl get services -n dnd-ai
kubectl logs -f deployment/bot -n dnd-ai
```

### Обновление образа

```bash
kubectl set image deployment/bot bot=dnd-bot:latest -n dnd-ai
kubectl rollout restart deployment/bot -n dnd-ai
```

### Масштабирование

```bash
kubectl scale deployment/bot --replicas=3 -n dnd-ai
```

**Важно:** Telegram long-polling бот должен обрабатывать обновления последовательно —
для горизонтального масштабирования нужен переход на webhook и/или очередь сообщений
(RabbitMQ, Redis), это не реализовано.

## Бэкапы и восстановление

```bash
make backup    # postgres_YYYYMMDD_HHMMSS.sql.gz + qdrant_YYYYMMDD_HHMMSS.tar.gz в ./backups/
               # старые бэкапы (>7 дней) удаляются автоматически

make restore POSTGRES_BACKUP=backups/postgres_xxx.sql.gz QDRANT_BACKUP=backups/qdrant_xxx.tar.gz
```

**ВНИМАНИЕ:** восстановление удаляет все текущие данные.

## Мониторинг

- `GET /health`, `GET /ready` — health/readiness бота (порт `8080`)
- LLM Monitoring UI и JSON API на `MONITORING_PORT` (по умолчанию `8081`) — прод-логи
  запросов/ответов GigaChat, счётчики `rag_empty_result_count`, `telegram_polling_error_count`,
  `output_leak_count` (см. `/api/metrics` и `.claude/skills/read-dnd-bot-logs/SKILL.md`)
- Логи с ротацией (10MB × 5 файлов, сжатие включено):

```bash
make prod-logs
# или
kubectl logs -f deployment/bot -n dnd-ai
```

Prometheus/Grafana/Alertmanager пока не подключены — если понадобится, начинать стоит
с уже собираемых `/api/metrics`.

## Безопасность

1. **Секреты:** не коммитить `.env`; использовать Kubernetes/Docker secrets; ротировать пароли и токены.
2. **Сеть:** внутренние сети Docker/Kubernetes, не открывать порты БД наружу, firewall-правила.
3. **Образы:** минимальные базовые образы, регулярные обновления, сканирование на уязвимости (`make security-scan`).
4. **Ресурсы:** лимиты выставлены в `build/docker-compose.prod.yml`/k8s-манифестах — не убирать при рефакторинге.

## CI/CD

`.github/workflows/`:
- **ci.yml** — тесты, линтинг, сборка образов при каждом push
- **deploy.yml** — деплой при push в `main` или теге `v*`
- **release.yml** — GitHub Release + changelog при теге `v*.*.*`

Секреты для деплоя (`DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY`) настраиваются в GitHub;
секция деплоя в `deploy.yml` включается под конкретную инфраструктуру. Подробнее о версионировании
образов и релизах — [VERSIONING.md](VERSIONING.md).

## Troubleshooting

**Бот не запускается:**
```bash
make prod-logs
docker compose -f build/docker-compose.prod.yml exec bot env | grep -E "TELEGRAM|GIGACHAT"
docker compose -f build/docker-compose.prod.yml exec postgres pg_isready -U dnd_user
```

**Проблемы с Qdrant:**
```bash
curl http://localhost:6334/healthz
docker compose -f build/docker-compose.prod.yml logs qdrant
```

**Проблемы с производительностью:**
```bash
docker stats
# или
kubectl top pods -n dnd-ai
```
Дальше — увеличить resource limits в конфигурации.

## Обновление

```bash
git pull
make prod-build
make prod-restart
```

Обновление Go-зависимостей: `go get -u ./... && go mod tidy`, затем пересобрать (`make prod-build`).
