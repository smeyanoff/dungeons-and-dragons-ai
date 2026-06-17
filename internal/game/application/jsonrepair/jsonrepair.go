package jsonrepair

import "strings"

// Clean удаляет markdown-блоки и лишний текст вокруг JSON.
func Clean(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
	} else if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
	}
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "```"))

	// Ищем начало JSON
	firstObj := strings.Index(raw, "{")
	firstArr := strings.Index(raw, "[")
	start := -1
	if firstObj >= 0 && firstArr >= 0 {
		if firstObj < firstArr {
			start = firstObj
		} else {
			start = firstArr
		}
	} else if firstObj >= 0 {
		start = firstObj
	} else if firstArr >= 0 {
		start = firstArr
	}
	if start > 0 {
		raw = raw[start:]
	}

	// Обрезаем постфикс после последнего закрывающего символа
	lastObj := strings.LastIndex(raw, "}")
	lastArr := strings.LastIndex(raw, "]")
	end := -1
	if lastObj >= 0 && lastArr >= 0 {
		if lastObj > lastArr {
			end = lastObj
		} else {
			end = lastArr
		}
	} else if lastObj >= 0 {
		end = lastObj
	} else if lastArr >= 0 {
		end = lastArr
	}
	if end >= 0 && end < len(raw)-1 {
		raw = raw[:end+1]
	}

	return strings.TrimSpace(raw)
}

// Repair пытается восстановить обрезанный JSON: удаляет висячие запятые и закрывает структуры.
func Repair(input string) string {
	jsonStr := strings.TrimSpace(input)
	if jsonStr == "" {
		return "{}"
	}

	jsonStr = trimTrailingCommas(jsonStr)

	openBraces := strings.Count(jsonStr, "{") - strings.Count(jsonStr, "}")
	openBrackets := strings.Count(jsonStr, "[") - strings.Count(jsonStr, "]")

	for i := 0; i < openBrackets; i++ {
		jsonStr += "]"
	}
	for i := 0; i < openBraces; i++ {
		jsonStr += "}"
	}

	return trimTrailingCommas(jsonStr)
}

func trimTrailingCommas(input string) string {
	out := input
	for {
		replaced := strings.ReplaceAll(out, ",}", "}")
		replaced = strings.ReplaceAll(replaced, ",]", "]")
		if replaced == out {
			return replaced
		}
		out = replaced
	}
}
