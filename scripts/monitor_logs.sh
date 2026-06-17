#!/bin/bash

# Скрипт для мониторинга логов контейнеров DnD бота и выявления проблем
# Использование: ./scripts/monitor_logs.sh

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROBLEMS_FILE="$PROJECT_ROOT/PROBLEMS_AND_BUGS.md"
CONTAINER_NAME="dnd-bot-prod"

# Цвета для вывода
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔍 Мониторинг логов контейнера: $CONTAINER_NAME${NC}"
echo -e "${YELLOW}Нажмите Ctrl+C для остановки${NC}"
echo ""

# Проверяем, запущен ли контейнер
if ! docker ps --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${RED}❌ Контейнер $CONTAINER_NAME не запущен!${NC}"
    exit 1
fi

# Функция для парсинга и анализа логов
analyze_logs() {
    local log_line="$1"
    
    # Проверка на критические ошибки
    if echo "$log_line" | grep -qiE "(ERROR|FATAL|PANIC)"; then
        echo -e "${RED}⚠️  ОБНАРУЖЕНА ОШИБКА:${NC}"
        echo "$log_line"
        echo ""
        
        # Проверяем типы ошибок
        if echo "$log_line" | grep -qiE "duplicate key.*idx_game_sessions_chat_id"; then
            echo -e "${YELLOW}📝 Это известная проблема #1: duplicate key при создании новой игры${NC}"
        elif echo "$log_line" | grep -qiE "GigaChat.*auth.*400|Can't decode.*Authorization"; then
            echo -e "${YELLOW}📝 Это известная проблема #2: GigaChat API аутентификация${NC}"
        elif echo "$log_line" | grep -qiE "unexpected EOF|connection reset"; then
            echo -e "${YELLOW}📝 Это известная проблема #4: Telegram API unexpected EOF${NC}"
        elif echo "$log_line" | grep -qiE "context deadline exceeded|timeout"; then
            echo -e "${YELLOW}📝 Проблема с таймаутами - требует проверки${NC}"
        elif echo "$log_line" | grep -qiE "SQLSTATE|database|postgres"; then
            echo -e "${YELLOW}📝 Проблема с базой данных - требует проверки${NC}"
        else
            echo -e "${YELLOW}📝 Новая ошибка - требует анализа${NC}"
        fi
        echo ""
    fi
    
    # Проверка на предупреждения
    if echo "$log_line" | grep -qiE "WARN.*Qdrant.*version.*not compatible"; then
        echo -e "${YELLOW}⚠️  Предупреждение: Несовместимость версий Qdrant (известная проблема #5)${NC}"
    fi
}

# Следим за логами в реальном времени
echo -e "${GREEN}📊 Начинаю мониторинг логов...${NC}"
echo ""

# Показываем последние 50 строк логов
echo -e "${YELLOW}--- Последние строки логов ---${NC}"
docker logs "$CONTAINER_NAME" --tail 50 2>&1 | while IFS= read -r line; do
    analyze_logs "$line"
done

echo ""
echo -e "${GREEN}--- Мониторинг в реальном времени ---${NC}"
echo ""

# Следим за новыми логами
docker logs "$CONTAINER_NAME" -f --tail 0 2>&1 | while IFS= read -r line; do
    echo "$line"
    analyze_logs "$line"
done
