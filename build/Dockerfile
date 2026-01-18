# Build stage
FROM golang:1.25-alpine AS builder

# Установка зависимостей для сборки
RUN apk add --no-cache git ca-certificates tzdata

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
# Используем поддерживаемую версию Alpine 3.19
FROM alpine:3.19

# Установка CA сертификатов для HTTPS запросов и wget для health check
RUN apk --no-cache add ca-certificates tzdata wget

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

# Expose порт для health check
EXPOSE 8080

# Запускаем приложение
CMD ["./bot"]