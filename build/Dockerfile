# Build stage
FROM golang:1.25-alpine AS builder

# Установка зависимостей для сборки
# Фиксируем версии пакетов для воспроизводимости сборок (golang:1.25-alpine)
# Регулярно обновляйте версии при обновлении базового образа
RUN apk add --no-cache \
    git=2.54.0-r0 \
    ca-certificates=20260611-r0 \
    tzdata=2026c-r0

# Рабочая директория
WORKDIR /app

# Копируем go mod файлы
COPY go.mod go.sum ./

# Загружаем зависимости (кэшируем для ускорения сборки)
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение с оптимизациями и версионированием
# Build args для версии (будут переданы из CI/CD)
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a \
    -installsuffix cgo \
    -ldflags="-w -s -extldflags \"-static\" -X dungeons-and-dragons-ai/pkg/version.Version=$VERSION -X dungeons-and-dragons-ai/pkg/version.Commit=$COMMIT -X dungeons-and-dragons-ai/pkg/version.BuildTime=$BUILD_TIME" \
    -o /app/bot \
    ./cmd/bot

# Final stage - минимальный alpine образ для health check
# Фиксируем версию Alpine для воспроизводимости сборок
# Обновлено до Alpine 3.20 для получения обновлений безопасности
FROM alpine:3.20

# Установка CA сертификатов для HTTPS запросов и wget для health check
# Фиксируем версии пакетов для воспроизводимости сборок (Alpine 3.20)
# Регулярно обновляйте версии при обновлении базового образа
RUN apk --no-cache add \
    ca-certificates=20260413-r0 \
    tzdata=2026b-r0 \
    wget=1.24.5-r0

WORKDIR /root/

# Копируем бинарник из builder stage
COPY --from=builder /app/bot .

# Копируем корневые CA сертификаты Sberbank для GigaChat API
# Контекст сборки - корень проекта, поэтому путь относительно корня
# Исключаем GOST 2025 сертификат (содержит несколько сертификатов, не поддерживается update-ca-certificates)
COPY build/certs/linux_russian_trusted_root_ca_pem/russian_trusted_root_ca_pem.crt /usr/local/share/ca-certificates/russian_trusted_root_ca_pem.crt

# Обновляем CA сертификаты (добавляем сертификаты Sberbank)
RUN update-ca-certificates

# Устанавливаем переменные окружения
ENV TZ=Europe/Moscow

# Expose ports for health check and monitoring UI
EXPOSE 8080
EXPOSE 8081

# Запускаем приложение
CMD ["./bot"]