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

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Prefer docker compose (v2); fallback to docker-compose (v1).
if docker compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

# Production compose file lives under build/
COMPOSE_FILE="build/docker-compose.prod.yml"

# Загрузка переменных окружения
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

BACKUP_DIR="./backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
POSTGRES_BACKUP_FILE="${BACKUP_DIR}/postgres_${TIMESTAMP}.sql"
QDRANT_BACKUP_DIR="${BACKUP_DIR}/qdrant_${TIMESTAMP}"

# Создание директории для бэкапов
mkdir -p ${BACKUP_DIR}

log_info "Начинаем создание бэкапов..."

# Бэкап PostgreSQL
log_info "Создание бэкапа PostgreSQL..."
$DOCKER_COMPOSE -f "$COMPOSE_FILE" exec -T postgres pg_dump -U ${POSTGRES_USER:-dnd_user} ${POSTGRES_DB:-dnd} > ${POSTGRES_BACKUP_FILE}
if [ $? -eq 0 ]; then
    log_info "Бэкап PostgreSQL создан: ${POSTGRES_BACKUP_FILE}"
    # Сжатие бэкапа
    gzip ${POSTGRES_BACKUP_FILE}
    log_info "Бэкап сжат: ${POSTGRES_BACKUP_FILE}.gz"
else
    log_error "Ошибка при создании бэкапа PostgreSQL"
    exit 1
fi

# Бэкап Qdrant (копирование данных)
log_info "Создание бэкапа Qdrant..."
$DOCKER_COMPOSE -f "$COMPOSE_FILE" exec -T qdrant tar czf - /qdrant/storage > ${QDRANT_BACKUP_DIR}.tar.gz
if [ $? -eq 0 ]; then
    log_info "Бэкап Qdrant создан: ${QDRANT_BACKUP_DIR}.tar.gz"
else
    log_error "Ошибка при создании бэкапа Qdrant"
    exit 1
fi

# Удаление старых бэкапов (старше 7 дней)
log_info "Удаление старых бэкапов (старше 7 дней)..."
find ${BACKUP_DIR} -name "*.gz" -type f -mtime +7 -delete
find ${BACKUP_DIR} -name "*.tar.gz" -type f -mtime +7 -delete

log_info "Бэкапы созданы успешно!"
log_info "PostgreSQL: ${POSTGRES_BACKUP_FILE}.gz"
log_info "Qdrant: ${QDRANT_BACKUP_DIR}.tar.gz"
