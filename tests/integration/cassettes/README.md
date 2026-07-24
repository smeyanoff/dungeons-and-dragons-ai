# LLM cassettes

Записанные пары запрос→ответ (LLM + RAG-эмбеддинги) для офлайн-воспроизведения real-LLM тестов —
см. «Cassette-тестирование (record/replay)» в `tests/integration/README.md` и механизм в
`tests/integration/llm_cassette_test.go`.

Файлы здесь — обычный JSON, коммитятся в git специально: чтобы другой разработчик или CI мог
прогнать `make test-telegram-replay` без сети и GigaChat credentials.

Пересоздать кассету после изменения промптов/тулзов:

```bash
set -a && source .env && set +a
make test-telegram-record CASSETTE=tests/integration/cassettes/<name>.json RUN=<TestName>
```
