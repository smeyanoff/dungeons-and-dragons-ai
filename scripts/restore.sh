#!/bin/bash

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Проверка аргументов
if [ $# -lt 2 ]; then
    log_error "Использование: $0 <postgres_backup_file> <qdrant_backup_file>"
    log_info "Пример: $0 backups/postgres_20240101_120000.sql.gz backups/qdrant_20240101_120000.tar.gz"
    exit 1
fi

POSTGRES_BACKUP=$1
QDRANT_BACKUP=$2

# Загрузка переменных окружения
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

log_warn "ВНИМАНИЕ: Восстановление из бэкапа удалит все текущие данные!"
read -p "Продолжить? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    log_info "Восстановление отменено"
    exit 0
fi

# Проверка существования файлов
if [ ! -f "$POSTGRES_BACKUP" ]; then
    log_error "Файл бэкапа PostgreSQL не найден: $POSTGRES_BACKUP"
    exit 1
fi

if [ ! -f "$QDRANT_BACKUP" ]; then
    log_error "Файл бэкапа Qdrant не найден: $QDRANT_BACKUP"
    exit 1
fi

log_info "Начинаем восстановление из бэкапов..."

# Остановка бота для безопасности
log_info "Остановка бота..."
docker-compose -f docker-compose.prod.yml stop bot || true

# Восстановление PostgreSQL
log_info "Восстановление PostgreSQL..."
if [[ "$POSTGRES_BACKUP" == *.gz ]]; then
    gunzip -c "$POSTGRES_BACKUP" | docker-compose -f docker-compose.prod.yml exec -T postgres psql -U ${POSTGRES_USER:-dnd_user} -d ${POSTGRES_DB:-dnd}
else
    cat "$POSTGRES_BACKUP" | docker-compose -f docker-compose.prod.yml exec -T postgres psql -U ${POSTGRES_USER:-dnd_user} -d ${POSTGRES_DB:-dnd}
fi

if [ $? -eq 0 ]; then
    log_info "PostgreSQL восстановлен успешно"
else
    log_error "Ошибка при восстановлении PostgreSQL"
    exit 1
fi

# Восстановление Qdrant
log_info "Восстановление Qdrant..."
docker-compose -f docker-compose.prod.yml stop qdrant
docker-compose -f docker-compose.prod.yml rm -f qdrant || true
docker-compose -f docker-compose.prod.yml up -d qdrant

# Ждем запуска Qdrant
sleep 10

cat "$QDRANT_BACKUP" | docker-compose -f docker-compose.prod.yml exec -T qdrant tar xzf - -C /

if [ $? -eq 0 ]; then
    log_info "Qdrant восстановлен успешно"
else
    log_error "Ошибка при восстановлении Qdrant"
    exit 1
fi

# Перезапуск бота
log_info "Перезапуск бота..."
docker-compose -f docker-compose.prod.yml restart bot

log_info "Восстановление завершено успешно!"
