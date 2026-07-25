// Package security содержит входной guard против попыток взлома игровой
// сессии и Мастера Игры (DM): prompt injection, jailbreak-фразы и попытки
// подделать протокол вызова инструментов прямо в реплике игрока.
//
// В отличие от output-guard'а (internal/telegram/bot_messages.go,
// sanitizePlayerFacingResponse в player_action), который чистит то, что
// LLM отдаёт игроку, этот guard проверяет то, что игрок отдаёт LLM —
// до того, как реплика попадёт в промпт DM, в промпт анализатора действий
// или будет проиндексирована в RAG (откуда могла бы "всплыть" повторно
// в будущих промптах).
package security

import "regexp"

// Result — результат проверки реплики игрока.
type Result struct {
	// Blocked — обнаружена явная попытка взлома (jailbreak-фраза).
	// Действие должно быть отклонено без обращения к LLM.
	Blocked bool
	// Reason — короткое машиночитаемое описание сработавшего правила
	// (для логов и метрик, не показывается игроку).
	Reason string
	// Sanitized — текст с обезвреженными протокольными тегами
	// (<tool_call>/<tool_result>). Всегда безопасен для дальнейшего
	// использования, даже если Blocked == false.
	Sanitized string
}

// protocolTagPattern вырезает попытки подделать теги протокола вызова
// инструментов прямо в реплике игрока. Если такой тег попадёт в промпт DM
// как часть "действия игрока", а модель его процитирует в своём ответе,
// tool_executor может распарсить и выполнить поддельный вызов инструмента.
var protocolTagPattern = regexp.MustCompile(`(?is)</?\s*(tool_call|tool_result|function_call)[^>]*>`)

// jailbreakPatterns — фразы-маркеры попыток переопределить роль модели,
// выйти за пределы игровой роли DM или выманить системные инструкции.
// Намеренно узкие (привязаны к мета-лексике "инструкции/промпт/роль"),
// чтобы не задевать обычные игровые реплики вроде "забудь про меч" или
// "ты теперь в подземелье".
// wordTail — окончание кириллического слова. Go regexp/RE2 не считает
// кириллицу частью \w (только [0-9A-Za-z_]), поэтому для склонений русских
// корней используется \p{L}* (любые буквы Unicode) вместо \w*.
const wordTail = `\p{L}*`

var jailbreakPatterns = []*regexp.Regexp{
	// RU: игнорирование/забывание инструкций и правил модели
	regexp.MustCompile(`(?i)игнорируй\s+(все\s+)?(предыдущ` + wordTail + `|систем` + wordTail + `)\s*(инструкц` + wordTail + `|правил` + wordTail + `|указан` + wordTail + `|команд` + wordTail + `)`),
	regexp.MustCompile(`(?i)забудь\s+(все\s+)?(предыдущ` + wordTail + `|систем` + wordTail + `|свои)\s*(инструкц` + wordTail + `|правил` + wordTail + `|команд` + wordTail + `)`),
	// RU: попытка сменить роль модели
	regexp.MustCompile(`(?i)ты\s+теперь\s+(не\s+)?(мастер\s+игры|ии[- ]?модель|ассистент|чат[- ]?бот|языков` + wordTail + `\s+модель)`),
	regexp.MustCompile(`(?i)забудь[,]?\s+что\s+ты\s+(мастер|дм|ии|ассистент)`),
	// RU: попытка выманить системный промпт/инструкции
	regexp.MustCompile(`(?i)(покажи|выведи|напиши|повтори|распечатай)\s+(мне\s+)?(свой\s+|весь\s+|полностью\s+)*(системн` + wordTail + `\s+)?(промпт|prompt)\b`),
	regexp.MustCompile(`(?i)систем` + wordTail + `\s+(промпт|инструкц` + wordTail + `)`),
	regexp.MustCompile(`(?i)повтори\s+(весь\s+)?текст\s+(выше|сверху|до\s+этого)`),
	// RU: режим без ограничений / режим разработчика
	regexp.MustCompile(`(?i)режим\s+разработчика`),
	regexp.MustCompile(`(?i)без\s+(каких[- ]?либо\s+)?ограничени[йя]\s+(отвечай|говори|действуй|веди)`),
	regexp.MustCompile(`(?i)(отвечай|говори|действуй|веди)\s+без\s+(каких[- ]?либо\s+)?ограничени[йя]`),
	// EN: классические jailbreak-формулировки
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(the\s+)?(previous|prior|above)\s+instructions`),
	regexp.MustCompile(`(?i)disregard\s+(all\s+)?(previous|prior|above)\s+(instructions|rules)`),
	regexp.MustCompile(`(?i)forget\s+(all\s+)?(your\s+)?(previous|prior)\s+instructions`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(a|an|no\s+longer)`),
	regexp.MustCompile(`(?i)reveal\s+(your\s+)?(system\s+)?prompt`),
	regexp.MustCompile(`(?i)(show|print|repeat)\s+(me\s+)?your\s+(system\s+)?(prompt|instructions)`),
	regexp.MustCompile(`(?i)developer\s+mode`),
	regexp.MustCompile(`(?i)act\s+as\s+(dan\b|an?\s+unrestricted)`),
	regexp.MustCompile(`(?i)\bai\s+language\s+model\b`),
	regexp.MustCompile(`(?i)\bsystem\s*:\s*\S`),
	regexp.MustCompile(`(?i)\bnew\s+instructions\s*:`),
}

// ScanPlayerInput проверяет реплику игрока перед тем, как она попадёт в
// промпт DM (BuildDMPrompt), в промпт анализатора действий
// (buildPlayerActionAnalysisPrompt) или в RAG-индекс. Всегда возвращает
// Sanitized-версию с вырезанными протокольными тегами; если реплика
// содержит явную jailbreak-фразу, дополнительно возвращает Blocked=true.
func ScanPlayerInput(text string) Result {
	sanitized := protocolTagPattern.ReplaceAllString(text, "")

	for _, p := range jailbreakPatterns {
		if p.MatchString(text) {
			return Result{
				Blocked:   true,
				Reason:    p.String(),
				Sanitized: sanitized,
			}
		}
	}

	return Result{Sanitized: sanitized}
}
