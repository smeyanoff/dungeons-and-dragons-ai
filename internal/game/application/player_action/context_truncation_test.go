package player_action

import (
	"fmt"
	"strings"
	"testing"
)

// Регрессия на баг: rag_context_builder.go переименовал/добавил заголовки блоков
// контекста ("Память этой локации", "Ключевые факты кампании", "Память этой локации
// (похожие моменты, найдено через поиск)"), но contextBlockPriority/summarizeBlockContent
// продолжали ссылаться на старые заголовки и резали новые блоки как безымянные (priority 3,
// грубый truncateMiddle) — из-за чего реальная история сообщений по локации могла быть
// вырезана из промпта раньше менее важных блоков.
func TestContextBlockPriority(t *testing.T) {
	tests := []struct {
		title string
		want  int
	}{
		{"Инструкции по проверкам навыков (ABILITY CHECKS)", 1},
		{"Текущая локация", 1},
		{"Текущий бой", 1},
		{"Ожидается проверка навыка", 1},
		{"Персонаж игрока", 2},
		{"Последние сообщения", 2},
		{"Память этой локации", 2},
		{"Ключевые факты кампании", 2},
		{"Инвентарь персонажа", 2},
		{"base", 2},
		{"Память этой локации (похожие моменты, найдено через поиск)", 3},
		{"RAG fallback", 3},
		{"какой-то неизвестный заголовок", 3},
		{"Игроки в сессии", 4},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			if got := contextBlockPriority(tt.title); got != tt.want {
				t.Errorf("contextBlockPriority(%q) = %d, want %d", tt.title, got, tt.want)
			}
		})
	}
}

func TestSummarizeBlockContent_LocationMemoryUsesRecentMessagesSummarizer(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("Игрок: сообщение номер %d про исследование пещеры", i))
	}
	content := "--- Память этой локации ---\n" + strings.Join(lines, "\n")

	// maxLen выбран так, чтобы уместить заголовок + 3 последних сообщения без
	// дополнительного слепого truncateMiddle поверх результата summarizeRecentMessages.
	got := summarizeBlockContent("Память этой локации", content, 350)

	if !strings.HasPrefix(got, "--- Память этой локации ---") {
		t.Errorf("summarizeBlockContent should keep the block header intact, got: %q", got)
	}
	if !strings.Contains(got, "сообщение номер 10") {
		t.Errorf("summarizeBlockContent should keep the most recent line, got: %q", got)
	}
	if strings.Contains(got, "...[context truncated]...") {
		t.Errorf("with enough room, location memory should be summarized structurally, not blindly middle-cut: %q", got)
	}
	if strings.Contains(got, "сообщение номер 1 ") {
		t.Errorf("summarizeBlockContent should drop old lines when summarizing, got: %q", got)
	}
}

func TestSummarizeBlockContent_LocationRAGUsesRAGSummarizer(t *testing.T) {
	content := "--- Память этой локации (похожие моменты, найдено через поиск) ---\n" +
		"[1] Игрок нашёл ключ в сундуке возле алтаря\n" +
		"[2] DM описал скрип старой двери в подземелье\n" +
		"[3] Игрок поговорил с призраком хранителя\n" +
		"[4] DM упомянул надпись на стене на древнем языке\n"

	// maxLen выбран так, чтобы уместить заголовок + 3 результата поиска без
	// дополнительного слепого truncateMiddle поверх результата summarizeRAGHistory.
	got := summarizeBlockContent("Память этой локации (похожие моменты, найдено через поиск)", content, 350)

	if !strings.HasPrefix(got, "--- Память этой локации (похожие моменты, найдено через поиск) ---") {
		t.Errorf("summarizeBlockContent should keep the block header intact, got: %q", got)
	}
	if strings.Contains(got, "...[context truncated]...") {
		t.Errorf("with enough room, RAG matches should be summarized structurally, not blindly middle-cut: %q", got)
	}
	if !strings.Contains(got, "[1]") || !strings.Contains(got, "[3]") {
		t.Errorf("summarizeRAGHistory should keep the top results, got: %q", got)
	}
	if strings.Contains(got, "[4]") {
		t.Errorf("summarizeRAGHistory should keep at most 3 results, got: %q", got)
	}
}

func TestTruncateContextByPriority_KeepsLocationMemoryOverLowerPriorityBlocks(t *testing.T) {
	repeat := func(line string, n int) string {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = fmt.Sprintf("%s %d", line, i)
		}
		return strings.Join(lines, "\n")
	}

	gameContext := strings.Join([]string{
		"Мир: Тестовый мир",
		"\n--- Текущая локация ---\nЛокация: Пещера с очень длинным описанием для теста",
		"\n--- Память этой локации ---\n" + repeat("Игрок: реплика про пещеру", 40),
		"\n--- Игроки в сессии ---\nКоличество игроков: 1, и это тоже длинный текст для проверки",
	}, "\n")

	// При этом maxLen после равномерного summarizeBlocks всё ещё чуть превышает лимит
	// (издержки на разделители между блоками), что заставляет алгоритм удалить ровно
	// один блок — блок с наименьшим приоритетом (наибольшим числом).
	const maxLen = 100

	result, removedBlocks, hardTruncated := truncateContextByPriority(gameContext, maxLen)

	if hardTruncated {
		t.Fatalf("did not expect hard truncation with these block sizes, result: %q", result)
	}

	removedSet := map[string]bool{}
	for _, title := range removedBlocks {
		removedSet[title] = true
	}
	if removedSet["Память этой локации"] {
		t.Errorf("priority-2 location memory block should not be removed while a lower-priority block remains, removed: %v", removedBlocks)
	}
	if !removedSet["Игроки в сессии"] {
		t.Errorf("lowest-priority block (Игроки в сессии, priority 4) should be removed first, removed: %v", removedBlocks)
	}
}
