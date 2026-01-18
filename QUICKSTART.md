# Быстрый старт для Production

## Минимальные требования

- Docker и Docker Compose
- 4GB RAM, 2 CPU cores
- 20GB свободного места на диске

## Шаги деплоя

### 1. Клонирование и настройка

```bash
git clone <repository-url>
cd dungeons-and-dragons-ai
cp .env.example .env
```

### 2. Настройка переменных окружения

Отредактируйте `.env` файл и укажите:

```bash
# Обязательные переменные
TELEGRAM_BOT_TOKEN=your_telegram_bot_token
GIGACHAT_CLIENT_ID=your_client_id
GIGACHAT_CLIENT_SECRET=your_client_secret
POSTGRES_PASSWORD=strong_password_here
```

### 3. Деплой

```bash
# Автоматический деплой (рекомендуется)
make deploy

# Или вручную:
docker-compose -f docker-compose.prod.yml build
docker-compose -f docker-compose.prod.yml up -d
```

### 4. Проверка

```bash
# Статус сервисов
make prod-ps

# Логи
make prod-logs

# Health check
curl http://localhost:8080/health
```

## Управление

```bash
# Просмотр логов
make prod-logs

# Перезапуск
make prod-restart

# Остановка
make prod-down

# Создание бэкапа
make backup
```

## Troubleshooting

### Бот не запускается

```bash
# Проверьте логи
make prod-logs

# Проверьте переменные окружения
docker-compose -f docker-compose.prod.yml exec bot env | grep TELEGRAM
```

### Проблемы с БД

```bash
# Проверьте статус PostgreSQL
docker-compose -f docker-compose.prod.yml exec postgres pg_isready -U dnd_user
```

### Проблемы с Qdrant

```bash
# Проверьте health check
curl http://localhost:6334/health
```

## Следующие шаги

Для более детальной информации см. [DEPLOY.md](DEPLOY.md)
