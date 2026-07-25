package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	llmdomain "dungeons-and-dragons-ai/internal/llm/domain"
	llmtools "dungeons-and-dragons-ai/internal/llm/domain/tools"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
)

// Record/replay ("кассеты") для интеграционных тестов с реальной LLM.
//
// Идея: один раз прогнать тест с реальным GigaChat (LLM_CASSETTE_MODE=record), сохранив каждый вызов
// LLM/эмбеддера в JSON-файл, а затем гонять тот же тест офлайн (LLM_CASSETTE_MODE=replay), без сети
// и credentials, воспроизводя записанные ответы. DM-промпт почти всегда включает RAG-контекст, поэтому
// эмбеддер кассетируется наравне с LLM — иначе текст prompt разойдётся между записью и воспроизведением
// и ключи перестанут совпадать. Сам векторный поиск (Qdrant) не кассетируется, но за счёт
// детерминированных (кассетированных) эмбеддингов возвращает тот же результат — при условии, что
// коллекция Qdrant не накопила посторонние точки от предыдущих прогонов (см. setupIntegrationTest).

// cassetteInteraction — одна записанная интеракция с LLM (Generate/GenerateWithMaxTokens/GenerateWithTools).
type cassetteInteraction struct {
	Seq           int                 `json:"seq"`
	Method        string              `json:"method"`
	Key           string              `json:"key"`
	PromptPreview string              `json:"prompt_preview"`
	MaxTokens     *int                `json:"max_tokens,omitempty"`
	ToolNames     []string            `json:"tool_names,omitempty"`
	Response      string              `json:"response,omitempty"`
	ToolCalls     []llmtools.ToolCall `json:"tool_calls,omitempty"`
	Finished      bool                `json:"finished,omitempty"`
	Error         string              `json:"error,omitempty"`
}

// embedInteraction — одна записанная интеракция с эмбеддером.
type embedInteraction struct {
	Seq         int       `json:"seq"`
	Key         string    `json:"key"`
	TextPreview string    `json:"text_preview"`
	Vector      []float32 `json:"vector,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// cassetteFile — содержимое одного кассетного JSON-файла (LLM + эмбеддинги вместе).
type cassetteFile struct {
	LLM   []cassetteInteraction `json:"llm"`
	Embed []embedInteraction    `json:"embed"`
}

func loadCassetteFile(path string) (*cassetteFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cassette file %s: %w", path, err)
	}
	var f cassetteFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to parse cassette file %s: %w", path, err)
	}
	return &f, nil
}

// previewText обрезает текст до maxRunes рун (не байт — сохраняет кириллицу читаемой в диагностике).
func previewText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// cassetteLLMKey вычисляет детерминированный ключ вызова LLM: одинаковый prompt/tools при записи
// и воспроизведении дают одинаковый ключ, независимо от порядка вызовов.
func cassetteLLMKey(method, prompt string, maxTokens *int, tools []llmtools.Tool) string {
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Name())
	}
	sort.Strings(toolNames)

	maxTokensStr := ""
	if maxTokens != nil {
		maxTokensStr = strconv.Itoa(*maxTokens)
	}

	canonical := method + "\n" + prompt + "\n" + maxTokensStr + "\n" + strings.Join(toolNames, ",")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func cassetteEmbedKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// cassetteWriter копит записанные интеракции в памяти и пишет JSON-файл целиком при каждом добавлении
// (просто и достаточно для локального тестового инструмента). Расшаривается между recordingLLM и
// recordingEmbedder, чтобы оба писали в один файл. Начинает работу с чистого листа — повторная запись
// полностью перезаписывает старую кассету (штатный способ обновить кассету после изменения промптов).
type cassetteWriter struct {
	mu   sync.Mutex
	path string
	file cassetteFile
}

func newCassetteWriter(path string) *cassetteWriter {
	return &cassetteWriter{path: path}
}

func (w *cassetteWriter) appendLLM(i cassetteInteraction) {
	w.mu.Lock()
	defer w.mu.Unlock()
	i.Seq = len(w.file.LLM)
	w.file.LLM = append(w.file.LLM, i)
	w.flushLocked()
}

func (w *cassetteWriter) appendEmbed(i embedInteraction) {
	w.mu.Lock()
	defer w.mu.Unlock()
	i.Seq = len(w.file.Embed)
	w.file.Embed = append(w.file.Embed, i)
	w.flushLocked()
}

func (w *cassetteWriter) flushLocked() {
	data, err := json.MarshalIndent(w.file, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return
	}
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, w.path)
}

// --- LLM: recording ---

type recordingLLM struct {
	inner  llmdomain.LLM
	writer *cassetteWriter
}

func newRecordingLLM(inner llmdomain.LLM, writer *cassetteWriter) llmdomain.LLM {
	return &recordingLLM{inner: inner, writer: writer}
}

func (r *recordingLLM) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := r.inner.Generate(ctx, prompt)
	interaction := cassetteInteraction{
		Method:        "Generate",
		Key:           cassetteLLMKey("Generate", prompt, nil, nil),
		PromptPreview: previewText(prompt, 200),
		Response:      resp,
	}
	if err != nil {
		interaction.Error = err.Error()
	}
	r.writer.appendLLM(interaction)
	return resp, err
}

func (r *recordingLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	resp, err := r.inner.GenerateWithMaxTokens(ctx, prompt, maxTokens)
	mt := maxTokens
	interaction := cassetteInteraction{
		Method:        "GenerateWithMaxTokens",
		Key:           cassetteLLMKey("GenerateWithMaxTokens", prompt, &mt, nil),
		PromptPreview: previewText(prompt, 200),
		MaxTokens:     &mt,
		Response:      resp,
	}
	if err != nil {
		interaction.Error = err.Error()
	}
	r.writer.appendLLM(interaction)
	return resp, err
}

func (r *recordingLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*llmdomain.LLMResponseWithTools, error) {
	resp, err := r.inner.GenerateWithTools(ctx, prompt, tools)

	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Name())
	}
	sort.Strings(toolNames)

	interaction := cassetteInteraction{
		Method:        "GenerateWithTools",
		Key:           cassetteLLMKey("GenerateWithTools", prompt, nil, tools),
		PromptPreview: previewText(prompt, 200),
		ToolNames:     toolNames,
	}
	if err != nil {
		interaction.Error = err.Error()
	} else if resp != nil {
		interaction.Response = resp.Content
		interaction.ToolCalls = resp.ToolCalls
		interaction.Finished = resp.Finished
	}
	r.writer.appendLLM(interaction)
	return resp, err
}

// --- LLM: replay ---

type replayingLLM struct {
	mu     sync.Mutex
	queues map[string][]cassetteInteraction
}

func newReplayingLLM(path string) (llmdomain.LLM, error) {
	f, err := loadCassetteFile(path)
	if err != nil {
		return nil, err
	}
	queues := make(map[string][]cassetteInteraction)
	for _, it := range f.LLM {
		queues[it.Key] = append(queues[it.Key], it)
	}
	return &replayingLLM{queues: queues}, nil
}

func (r *replayingLLM) pop(key, method, promptPreview string) (*cassetteInteraction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := r.queues[key]
	if len(q) == 0 {
		return nil, fmt.Errorf(
			"cassette miss: нет записанного вызова %s (key=%s, prompt=%q). Пересоздайте кассету: LLM_CASSETTE_MODE=record",
			method, key, promptPreview,
		)
	}
	rec := q[0]
	r.queues[key] = q[1:]
	return &rec, nil
}

func (r *replayingLLM) Generate(ctx context.Context, prompt string) (string, error) {
	key := cassetteLLMKey("Generate", prompt, nil, nil)
	rec, err := r.pop(key, "Generate", previewText(prompt, 200))
	if err != nil {
		return "", err
	}
	if rec.Error != "" {
		return "", errors.New(rec.Error)
	}
	return rec.Response, nil
}

func (r *replayingLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	mt := maxTokens
	key := cassetteLLMKey("GenerateWithMaxTokens", prompt, &mt, nil)
	rec, err := r.pop(key, "GenerateWithMaxTokens", previewText(prompt, 200))
	if err != nil {
		return "", err
	}
	if rec.Error != "" {
		return "", errors.New(rec.Error)
	}
	return rec.Response, nil
}

func (r *replayingLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*llmdomain.LLMResponseWithTools, error) {
	key := cassetteLLMKey("GenerateWithTools", prompt, nil, tools)
	rec, err := r.pop(key, "GenerateWithTools", previewText(prompt, 200))
	if err != nil {
		return nil, err
	}
	if rec.Error != "" {
		return nil, errors.New(rec.Error)
	}
	return &llmdomain.LLMResponseWithTools{
		Content:   rec.Response,
		ToolCalls: rec.ToolCalls,
		Finished:  rec.Finished,
	}, nil
}

// --- Embedder: recording/replay ---

type recordingEmbedder struct {
	inner  ragdomain.Embedder
	writer *cassetteWriter
}

func newRecordingEmbedder(inner ragdomain.Embedder, writer *cassetteWriter) ragdomain.Embedder {
	return &recordingEmbedder{inner: inner, writer: writer}
}

func (e *recordingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vec, err := e.inner.Embed(ctx, text)
	interaction := embedInteraction{
		Key:         cassetteEmbedKey(text),
		TextPreview: previewText(text, 200),
		Vector:      vec,
	}
	if err != nil {
		interaction.Error = err.Error()
	}
	e.writer.appendEmbed(interaction)
	return vec, err
}

type replayingEmbedder struct {
	mu     sync.Mutex
	queues map[string][]embedInteraction
}

func newReplayingEmbedder(path string) (ragdomain.Embedder, error) {
	f, err := loadCassetteFile(path)
	if err != nil {
		return nil, err
	}
	queues := make(map[string][]embedInteraction)
	for _, it := range f.Embed {
		queues[it.Key] = append(queues[it.Key], it)
	}
	return &replayingEmbedder{queues: queues}, nil
}

func (e *replayingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	key := cassetteEmbedKey(text)
	e.mu.Lock()
	q := e.queues[key]
	if len(q) == 0 {
		e.mu.Unlock()
		return nil, fmt.Errorf(
			"embed cassette miss: нет записанного эмбеддинга (key=%s, text=%q). Пересоздайте кассету: LLM_CASSETTE_MODE=record",
			key, previewText(text, 200),
		)
	}
	rec := q[0]
	e.queues[key] = q[1:]
	e.mu.Unlock()
	if rec.Error != "" {
		return nil, errors.New(rec.Error)
	}
	return rec.Vector, nil
}

// --- Тесты самого механизма (без БД/Qdrant/сети) ---

type fakeCassetteTool struct{ name string }

func (f fakeCassetteTool) Name() string                                                { return f.name }
func (f fakeCassetteTool) Description() string                                         { return "fake tool for cassette tests" }
func (f fakeCassetteTool) Parameters() json.RawMessage                                 { return json.RawMessage(`{}`) }
func (f fakeCassetteTool) Execute(context.Context, map[string]interface{}) (interface{}, error) {
	return nil, nil
}

type fakeInnerLLM struct {
	generateCalls     int
	genWithToolsCalls int
}

func (f *fakeInnerLLM) Generate(ctx context.Context, prompt string) (string, error) {
	f.generateCalls++
	return "response to: " + prompt, nil
}

func (f *fakeInnerLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	return "response with max tokens", nil
}

func (f *fakeInnerLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*llmdomain.LLMResponseWithTools, error) {
	f.genWithToolsCalls++
	return &llmdomain.LLMResponseWithTools{
		Content:   "tool response",
		ToolCalls: []llmtools.ToolCall{{Name: "roll_dice", Arguments: map[string]interface{}{"dice_expression": "d20"}}},
		Finished:  false,
	}, nil
}

type fakeInnerEmbedder struct{ calls int }

func (f *fakeInnerEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	f.calls++
	return []float32{0.1, 0.2, 0.3}, nil
}

func TestCassetteLLMKey_Deterministic(t *testing.T) {
	tools := []llmtools.Tool{fakeCassetteTool{name: "b"}, fakeCassetteTool{name: "a"}}
	toolsReordered := []llmtools.Tool{fakeCassetteTool{name: "a"}, fakeCassetteTool{name: "b"}}

	k1 := cassetteLLMKey("GenerateWithTools", "hello", nil, tools)
	k2 := cassetteLLMKey("GenerateWithTools", "hello", nil, toolsReordered)
	if k1 != k2 {
		t.Fatalf("expected same key regardless of tool order, got %s vs %s", k1, k2)
	}

	k3 := cassetteLLMKey("GenerateWithTools", "different prompt", nil, tools)
	if k1 == k3 {
		t.Fatal("expected different key for different prompt")
	}

	mt := 100
	k4 := cassetteLLMKey("GenerateWithMaxTokens", "hello", &mt, nil)
	k5 := cassetteLLMKey("GenerateWithMaxTokens", "hello", nil, nil)
	if k4 == k5 {
		t.Fatal("expected different key when maxTokens differs")
	}
}

func TestCassette_RecordThenReplay(t *testing.T) {
	dir := t.TempDir()
	cassettePath := filepath.Join(dir, "cassette.json")
	ctx := context.Background()

	// --- Record: реальные (тут — фейковые) вызовы идут через recordingLLM/recordingEmbedder ---
	innerLLM := &fakeInnerLLM{}
	innerEmbedder := &fakeInnerEmbedder{}
	writer := newCassetteWriter(cassettePath)
	recLLM := newRecordingLLM(innerLLM, writer)
	recEmbedder := newRecordingEmbedder(innerEmbedder, writer)

	genResp, err := recLLM.Generate(ctx, "Привет, мир")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	tools := []llmtools.Tool{fakeCassetteTool{name: "roll_dice"}}
	toolsResp, err := recLLM.GenerateWithTools(ctx, "Используй тулзы", tools)
	if err != nil {
		t.Fatalf("GenerateWithTools: %v", err)
	}

	vec, err := recEmbedder.Embed(ctx, "текст для эмбеддинга")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if innerLLM.generateCalls != 1 || innerLLM.genWithToolsCalls != 1 {
		t.Fatalf("expected inner LLM to be called once per method, got %+v", innerLLM)
	}
	if innerEmbedder.calls != 1 {
		t.Fatalf("expected inner embedder to be called once, got %d", innerEmbedder.calls)
	}
	if _, err := os.Stat(cassettePath); err != nil {
		t.Fatalf("expected cassette file to be written: %v", err)
	}

	// --- Replay: полностью офлайн, inner-реализации не используются вообще ---
	replayLLM, err := newReplayingLLM(cassettePath)
	if err != nil {
		t.Fatalf("newReplayingLLM: %v", err)
	}
	replayEmbedder, err := newReplayingEmbedder(cassettePath)
	if err != nil {
		t.Fatalf("newReplayingEmbedder: %v", err)
	}

	replayedGen, err := replayLLM.Generate(ctx, "Привет, мир")
	if err != nil {
		t.Fatalf("replay Generate: %v", err)
	}
	if replayedGen != genResp {
		t.Fatalf("replayed Generate mismatch: got %q want %q", replayedGen, genResp)
	}

	replayedTools, err := replayLLM.GenerateWithTools(ctx, "Используй тулзы", tools)
	if err != nil {
		t.Fatalf("replay GenerateWithTools: %v", err)
	}
	if replayedTools.Content != toolsResp.Content || len(replayedTools.ToolCalls) != len(toolsResp.ToolCalls) {
		t.Fatalf("replayed GenerateWithTools mismatch: got %+v want %+v", replayedTools, toolsResp)
	}

	replayedVec, err := replayEmbedder.Embed(ctx, "текст для эмбеддинга")
	if err != nil {
		t.Fatalf("replay Embed: %v", err)
	}
	if len(replayedVec) != len(vec) {
		t.Fatalf("replayed embedding length mismatch: got %d want %d", len(replayedVec), len(vec))
	}
}

func TestCassette_ReplayMissReturnsError(t *testing.T) {
	dir := t.TempDir()
	cassettePath := filepath.Join(dir, "empty_cassette.json")
	if err := os.WriteFile(cassettePath, []byte(`{"llm":[],"embed":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write empty cassette: %v", err)
	}

	replayLLM, err := newReplayingLLM(cassettePath)
	if err != nil {
		t.Fatalf("newReplayingLLM: %v", err)
	}

	_, err = replayLLM.Generate(context.Background(), "unseen prompt")
	if err == nil {
		t.Fatal("expected cassette miss error, got nil")
	}
	if !strings.Contains(err.Error(), "cassette miss") {
		t.Fatalf("expected cassette miss error, got: %v", err)
	}
}

func TestCassette_ReplayMissingFile(t *testing.T) {
	if _, err := newReplayingLLM("/nonexistent/path/cassette.json"); err == nil {
		t.Fatal("expected error when cassette file does not exist")
	}
}
