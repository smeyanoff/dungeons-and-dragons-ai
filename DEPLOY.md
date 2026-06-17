# Руководство по деплою в Production

Это руководство описывает процесс развертывания D&D AI бота в production окружении.

## Предварительные требования

- Docker и Docker Compose установлены
- Доступ к серверу с достаточными ресурсами (минимум 4GB RAM, 2 CPU cores)
- Настроенные переменные окружения (см. `.env.example`)
- Доступ к GigaChat API
- Telegram Bot Token

## Быстрый старт

### 1. Подготовка окружения

```bash
# Клонируйте репозиторий
git clone <repository-url>
cd dungeons-and-dragons-ai

# Создайте .env файл на основе примера
cp .env.example .env

# Отредактируйте .env и заполните все необходимые значения
nano .env
```

### 2. Деплой через Docker Compose

```bash
# Используйте готовый скрипт деплоя
make deploy

# Или вручную:
docker-compose -f docker-compose.prod.yml build
docker-compose -f docker-compose.prod.yml up -d
```

### 3. Проверка статуса

```bash
# Проверка статуса сервисов
make prod-ps

# Просмотр логов
make prod-logs

# Проверка health check
curl http://localhost:8080/health
```

## Структура Production конфигурации

### docker-compose.prod.yml

Production версия docker-compose с улучшенными настройками:
- Resource limits для всех сервисов
- Health checks
- Security settings (no-new-privileges, read-only файловая система)
- Оптимизированное логирование
- Graceful shutdown

### Переменные окружения

Все переменные должны быть установлены в `.env` файле:

**Обязательные:**
- `TELEGRAM_BOT_TOKEN` - токен Telegram бота
- `GIGACHAT_CLIENT_ID` - Client ID для GigaChat API
- `GIGACHAT_CLIENT_SECRET` - Client Secret для GigaChat API
- `POSTGRES_PASSWORD` - сильный пароль для PostgreSQL

**Опциональные:**
- `POSTGRES_USER` - пользователь БД (по умолчанию: `dnd_user`)
- `POSTGRES_DB` - имя БД (по умолчанию: `dnd`)
- `GIGACHAT_MODEL` - модель GigaChat (по умолчанию: `GigaChat`)
- `LOG_LEVEL` - уровень логирования (по умолчанию: `info`)

## Деплой в Kubernetes

### Предварительные требования

- Kubernetes кластер (версия 1.20+)
- kubectl настроен и подключен к кластеру
- Доступ к Container Registry (для хранения образов)

### Шаги деплоя

1. **Создайте namespace:**

```bash
kubectl apply -f k8s/namespace.yaml
```

2. **Настройте секреты:**

Отредактируйте секреты в `k8s/postgres.yaml` и `k8s/bot.yaml`:

```bash
# Создайте секреты вручную или отредактируйте манифесты
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
```

3. **Деплой сервисов:**

```bash
# PostgreSQL
kubectl apply -f k8s/postgres.yaml

# Qdrant
kubectl apply -f k8s/qdrant.yaml

# Bot
kubectl apply -f k8s/bot.yaml
```

4. **Проверка статуса:**

```bash
kubectl get pods -n dnd-ai
kubectl get services -n dnd-ai
kubectl logs -f deployment/bot -n dnd-ai
```

### Обновление образа

```bash
# Обновите образ в манифесте или используйте:
kubectl set image deployment/bot bot=dnd-bot:latest -n dnd-ai
kubectl rollout restart deployment/bot -n dnd-ai
```

## Бэкапы и восстановление

### Создание бэкапа

```bash
# Автоматический бэкап
make backup

# Или вручную:
./scripts/backup.sh
```

Бэкапы сохраняются в директории `./backups/`:
- PostgreSQL: `postgres_YYYYMMDD_HHMMSS.sql.gz`
- Qdrant: `qdrant_YYYYMMDD_HHMMSS.tar.gz`

Старые бэкапы (старше 7 дней) автоматически удаляются.

### Восстановление из бэкапа

```bash
make restore POSTGRES_BACKUP=backups/postgres_20240101_120000.sql.gz QDRANT_BACKUP=backups/qdrant_20240101_120000.tar.gz

# Или вручную:
./scripts/restore.sh backups/postgres_xxx.sql.gz backups/qdrant_xxx.tar.gz
```

**ВНИМАНИЕ:** Восстановление удалит все текущие данные!

## Мониторинг

### Health Checks

Приложение предоставляет два endpoint:

- `GET /health` - проверка здоровья (проверяет БД и Qdrant)
- `GET /ready` - проверка готовности

```bash
# Проверка здоровья
curl http://localhost:8080/health

# Проверка готовности
curl http://localhost:8080/ready
```

### Логирование

Логи доступны через:

```bash
# Docker Compose
docker-compose -f docker-compose.prod.yml logs -f bot

# Kubernetes
kubectl logs -f deployment/bot -n dnd-ai
```

Логи настроены на ротацию:
- Максимальный размер файла: 10MB
- Количество файлов: 5
- Сжатие: включено

### Метрики (будущее расширение)

Для production рекомендуется добавить:
- Prometheus для сбора метрик
- Grafana для визуализации
- Alertmanager для алертов

## CI/CD

### GitHub Actions

Проект включает CI/CD конфигурацию в `.github/workflows/`:

- **ci.yml** - автоматические тесты и сборка при каждом push
- **deploy.yml** - автоматический деплой при push в main или создании тега

### Настройка CI/CD

1. Убедитесь, что секреты настроены в GitHub:
   - `DEPLOY_HOST` - хост для деплоя
   - `DEPLOY_USER` - пользователь для SSH
   - `DEPLOY_SSH_KEY` - SSH ключ для доступа

2. Раскомментируйте секцию деплоя в `.github/workflows/deploy.yml`

3. Настройте под вашу инфраструктуру

## Безопасность

### Рекомендации для Production

1. **Секреты:**
   - Никогда не коммитьте `.env` файл
   - Используйте секреты Kubernetes или Docker secrets
   - Ротация паролей и токенов

2. **Сеть:**
   - Используйте внутренние сети Docker/Kubernetes
   - Не экспортируйте порты БД наружу
   - Используйте firewall правила

3. **Образы:**
   - Используйте минимальные базовые образы
   - Регулярно обновляйте базовые образы
   - Сканируйте образы на уязвимости

4. **Ресурсы:**
   - Установите лимиты ресурсов
   - Мониторьте использование ресурсов

## Масштабирование

### Горизонтальное масштабирование

Для масштабирования бота в Kubernetes:

```bash
kubectl scale deployment/bot --replicas=3 -n dnd-ai
```

**Важно:** Telegram бот должен обрабатывать обновления последовательно. Для масштабирования может потребоваться:
- Использование webhook вместо long polling
- Очередь сообщений (RabbitMQ, Redis)
- Балансировка нагрузки

### Вертикальное масштабирование

Обновите resource limits в `docker-compose.prod.yml` или Kubernetes манифестах.

## Troubleshooting

### Бот не запускается

1. Проверьте логи:
```bash
make prod-logs
```

2. Проверьте переменные окружения:
```bash
docker-compose -f docker-compose.prod.yml exec bot env | grep -E "TELEGRAM|GIGACHAT"
```

3. Проверьте подключение к БД:
```bash
docker-compose -f docker-compose.prod.yml exec postgres pg_isready -U dnd_user
```

### Проблемы с Qdrant

1. Проверьте статус:
```bash
curl http://localhost:6334/healthz
```

2. Проверьте логи:
```bash
docker-compose -f docker-compose.prod.yml logs qdrant
```

### Проблемы с производительностью

1. Проверьте использование ресурсов:
```bash
docker stats
# или
kubectl top pods -n dnd-ai
```

2. Увеличьте лимиты ресурсов в конфигурации

## Обновление

### Обновление приложения

```bash
# Получите последние изменения
git pull

# Пересоберите образы
make prod-build

# Перезапустите сервисы
make prod-restart
```

### Обновление зависимостей

```bash
# Обновите Go зависимости
go get -u ./...
go mod tidy

# Обновите версии в docker-compose.prod.yml
# Пересоберите образы
make prod-build
```

## Поддержка

При возникновении проблем:
1. Проверьте логи
2. Проверьте health checks
3. Проверьте документацию
4. Создайте issue в репозитории
