#!/bin/bash

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Функция для вывода сообщений
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Проверка наличия необходимых команд
check_command() {
    if ! command -v "$1" &> /dev/null; then
        log_error "$1 не установлен! Установите $1 для продолжения."
        exit 1
    fi
}

log_info "Проверка необходимых команд..."
check_command docker
check_command docker-compose

# Проверка наличия .env файла
if [ ! -f .env ]; then
    log_error ".env файл не найден!"
    log_info "Скопируйте .env.example в .env и заполните необходимые значения"
    exit 1
fi

# Безопасная загрузка переменных окружения из .env
log_info "Загрузка переменных окружения из .env..."
set -a
source .env
set +a

# Проверка обязательных переменных
REQUIRED_VARS=("TELEGRAM_BOT_TOKEN" "GIGACHAT_CLIENT_ID" "GIGACHAT_CLIENT_SECRET" "POSTGRES_PASSWORD")
for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
        log_error "Переменная окружения $var не установлена!"
        exit 1
    fi
done

log_info "Начинаем деплой в production..."

# Создание директорий для данных
log_info "Создание директорий для данных..."
mkdir -p data/postgres data/qdrant backups

# Остановка старых контейнеров
log_info "Остановка старых контейнеров..."
docker-compose -f build/docker-compose.prod.yml down || true

# Получение версии и коммита для build args
# Для production используем версию "prod", если не указана явно
VERSION=${VERSION:-prod}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# Экспорт переменных для docker-compose
export VERSION COMMIT BUILD_TIME

# Сборка образов
log_info "Сборка Docker образов с версией: $VERSION, коммит: $COMMIT, build time: $BUILD_TIME..."
docker-compose -f build/docker-compose.prod.yml build

# Запуск сервисов
log_info "Запуск сервисов..."
docker-compose -f build/docker-compose.prod.yml up -d

# Ожидание готовности сервисов
log_info "Ожидание готовности сервисов..."
sleep 10

# Проверка статуса
log_info "Проверка статуса сервисов..."
docker-compose -f build/docker-compose.prod.yml ps

# Проверка здоровья сервисов
log_info "Проверка здоровья сервисов..."

# Установка значений по умолчанию для переменных
POSTGRES_USER=${POSTGRES_USER:-dnd_user}
POSTGRES_DB=${POSTGRES_DB:-dnd}

# Проверка PostgreSQL
log_info "Ожидание готовности PostgreSQL..."
for i in $(seq 1 30); do
    if docker-compose -f build/docker-compose.prod.yml exec -T postgres pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" > /dev/null 2>&1; then
        log_info "PostgreSQL готов"
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "PostgreSQL не готов после 30 попыток"
        log_error "Проверьте логи: docker-compose -f build/docker-compose.prod.yml logs postgres"
        exit 1
    fi
    sleep 2
done

# Проверка Qdrant
log_info "Ожидание готовности Qdrant..."
# Используем curl из хоста, так как в контейнере Qdrant может не быть wget
for i in $(seq 1 30); do
    # Проверяем через health check статус контейнера или через curl с хоста
    if curl -sf http://localhost:6334/health > /dev/null 2>&1 || \
       docker inspect dnd-qdrant-prod --format='{{.State.Health.Status}}' 2>/dev/null | grep -q "healthy"; then
        log_info "Qdrant готов"
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Qdrant не готов после 30 попыток"
        log_error "Проверьте логи: docker-compose -f build/docker-compose.prod.yml logs qdrant"
        log_error "Проверьте статус: docker inspect dnd-qdrant-prod --format='{{.State.Health.Status}}'"
        exit 1
    fi
    sleep 2
done

# Проверка бота (health check endpoint)
log_info "Ожидание готовности бота..."
for i in $(seq 1 30); do
    if docker-compose -f build/docker-compose.prod.yml exec -T bot wget --no-verbose --tries=1 --spider http://localhost:8080/health > /dev/null 2>&1; then
        log_info "Бот готов"
        break
    fi
    if [ $i -eq 30 ]; then
        log_warn "Бот не отвечает на health check после 30 попыток"
        log_warn "Проверьте логи: docker-compose -f build/docker-compose.prod.yml logs bot"
        # Не выходим с ошибкой, так как бот может запускаться дольше
    fi
    sleep 2
done

# Финальная проверка статуса
log_info "Финальная проверка статуса сервисов..."
if ! docker-compose -f build/docker-compose.prod.yml ps | grep -q "healthy\|Up"; then
    log_warn "Некоторые сервисы могут быть не готовы. Проверьте статус:"
    docker-compose -f build/docker-compose.prod.yml ps
fi

# Просмотр логов бота
log_info "Просмотр последних логов бота..."
docker-compose -f build/docker-compose.prod.yml logs --tail=50 bot

# Проверка версии приложения
log_info "Проверка версии приложения..."
if docker-compose -f build/docker-compose.prod.yml exec -T bot wget -q -O- http://localhost:8080/version 2>/dev/null; then
    log_info "Версия приложения получена успешно"
else
    log_warn "Не удалось получить версию приложения (возможно, бот еще запускается)"
fi

log_info "Деплой завершен успешно!"
log_info "Для просмотра логов используйте: docker-compose -f build/docker-compose.prod.yml logs -f bot"
log_info "Для проверки статуса используйте: docker-compose -f build/docker-compose.prod.yml ps"