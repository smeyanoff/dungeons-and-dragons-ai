# Версионирование

Этот документ описывает систему версионирования приложения.

## Формат версий

Проект использует [Semantic Versioning](https://semver.org/):
- **MAJOR** - несовместимые изменения API
- **MINOR** - обратно совместимые новые функции
- **PATCH** - обратно совместимые исправления ошибок

Формат: `vMAJOR.MINOR.PATCH` (например, `v1.2.3`)

## Версия в приложении

Версия внедряется в приложение при сборке через build flags:

```go
// pkg/version/version.go
var (
    Version = "dev"      // Версия приложения
    Commit = "unknown"   // Git commit SHA
    BuildTime = "unknown" // Время сборки
)
```

### Проверка версии

```bash
curl http://localhost:8080/version
```

```json
{
  "version": "v1.2.3",
  "commit": "abc123",
  "buildTime": "2024-01-01T12:00:00Z",
  "goVersion": "go1.25.5"
}
```

Версия также выводится в логах при старте приложения (`Starting bot... Version: v1.2.3, ...`)
и доступна программно:

```go
import "dungeons-and-dragons-ai/pkg/version"

ver := version.Get()
fmt.Printf("Version: %s\n", ver.Version)
```

## Версионирование образов Docker

### Автоматическое версионирование

CI/CD автоматически создает теги для Docker образов:

- **Для тегов** (`v1.2.3`):
  - `v1.2.3`
  - `1.2.3`
  - `1.2`
  - `latest` (только для последнего тега)

- **Для веток** (`main`, `develop`):
  - `main-abc123` (branch-sha)
  - `main` (latest из ветки)

- **Для PR**:
  - `pr-123-abc123`

### Ручная сборка

При локальной сборке можно указать версию:

```bash
make build VERSION=v1.0.0 COMMIT=$(git rev-parse --short HEAD) BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
```

Или для Docker:

```bash
make prod-build VERSION=v1.0.0 COMMIT=$(git rev-parse --short HEAD) BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
```

## Создание релиза

### 1. Подготовка

```bash
# Убедитесь, что все изменения закоммичены
git status

# Обновите версию в документации (если нужно)
```

### 2. Создание тега

```bash
# Создайте тег с версией
git tag -a v1.2.3 -m "Release v1.2.3: Описание изменений"

# Отправьте тег в репозиторий
git push origin v1.2.3
```

### 3. Автоматический релиз

После создания тега GitHub Actions автоматически:

1. Соберет Docker образ с версией
2. Запустит тесты
3. Создаст GitHub Release
4. Опубликует образ в Container Registry

### 4. Проверка

Проверьте релиз:

```bash
# Проверьте образ
docker pull ghcr.io/your-org/dungeons-and-dragons-ai:v1.2.3

# Проверьте версию в контейнере
docker run --rm ghcr.io/your-org/dungeons-and-dragons-ai:v1.2.3 /bot --version
```

## Workflow для релизов

### CI Pipeline (`.github/workflows/ci.yml`)

- Триггер: push в `main`, `develop` или PR
- Создает образы с версией на основе ветки/PR
- Не создает GitHub Release

### Deploy Pipeline (`.github/workflows/deploy.yml`)

- Триггер: push в `main` или теги `v*`
- Создает production образы
- Тегирует как `latest` для `main`

### Release Pipeline (`.github/workflows/release.yml`)

- Триггер: теги `v*.*.*`
- Создает полный релиз с:
  - Docker образами
  - GitHub Release с changelog
  - Бинарными файлами

## Changelog

Changelog генерируется автоматически на основе коммитов между тегами.

Для лучшего changelog используйте [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` - новая функция
- `fix:` - исправление ошибки
- `docs:` - изменения документации
- `refactor:` - рефакторинг
- `test:` - добавление тестов
- `chore:` - рутинные задачи

Пример:

```bash
git commit -m "feat: добавить поддержку квестов"
git commit -m "fix: исправить ошибку в combat system"
```

## Обновление версии

### Patch версия (1.2.3 -> 1.2.4)

Для исправлений ошибок:

```bash
git tag -a v1.2.4 -m "Fix: описание исправления"
git push origin v1.2.4
```

### Minor версия (1.2.3 -> 1.3.0)

Для новых функций (обратно совместимых):

```bash
git tag -a v1.3.0 -m "Feature: описание новых функций"
git push origin v1.3.0
```

### Major версия (1.2.3 -> 2.0.0)

Для несовместимых изменений:

```bash
git tag -a v2.0.0 -m "Breaking: описание изменений"
git push origin v2.0.0
```

## Troubleshooting

### Версия показывает "dev"

Это означает, что приложение собрано без указания версии. Проверьте:

1. Используется ли правильный Dockerfile с build args
2. Передаются ли build args в CI/CD
3. Используется ли правильная команда сборки

### Версия не обновляется

Убедитесь, что:

1. Тег создан правильно (`v1.2.3`)
2. Тег отправлен в репозиторий
3. CI/CD pipeline запустился
4. Образ пересобран с новыми build args

## Рекомендации

1. **Используйте теги для релизов** - не полагайтесь на `latest`
2. **Документируйте breaking changes** в сообщении тега
3. **Используйте Semantic Versioning** последовательно
4. **Тестируйте версию** перед релизом
5. **Обновляйте CHANGELOG.md** при необходимости (если используется)
