package dice

import (
	"crypto/rand"
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
)

// RollResult представляет результат броска кубика
type RollResult struct {
	Expression string // Исходное выражение (например, "2d6+3")
	Rolls      []int  // Отдельные броски
	Total      int    // Итоговая сумма
	Modifier   int    // Модификатор (если был)
}

// Roll выполняет бросок кубика по стандартной нотации D&D
// Поддерживает форматы: "d20", "1d6", "2d6+3", "1d8-1", "3d4+5"
func Roll(expression string) (*RollResult, error) {
	// Нормализуем выражение
	expr := strings.ToLower(strings.ReplaceAll(expression, " ", ""))

	// Парсим выражение вида: [количество]d[стороны][+/-модификатор]
	// Примеры: "d20", "1d6", "2d6+3", "1d8-1"
	re := regexp.MustCompile(`^(\d*)d(\d+)([+-]\d+)?$`)
	matches := re.FindStringSubmatch(expr)

	if len(matches) == 0 {
		return nil, &InvalidDiceExpressionError{Expression: expression}
	}

	// Количество кубиков (по умолчанию 1)
	count := 1
	if matches[1] != "" {
		var err error
		count, err = strconv.Atoi(matches[1])
		if err != nil || count <= 0 {
			return nil, &InvalidDiceExpressionError{Expression: expression}
		}
	}

	// Количество сторон
	sides, err := strconv.Atoi(matches[2])
	if err != nil || sides <= 0 {
		return nil, &InvalidDiceExpressionError{Expression: expression}
	}

	// Модификатор (опционально)
	modifier := 0
	if matches[3] != "" {
		modifier, err = strconv.Atoi(matches[3])
		if err != nil {
			return nil, &InvalidDiceExpressionError{Expression: expression}
		}
	}

	// Генерируем случайные числа используя криптографически стойкий генератор
	rolls := make([]int, count)
	total := 0

	for i := 0; i < count; i++ {
		rolls[i] = secureRandomInt(sides) + 1
		total += rolls[i]
	}

	total += modifier

	return &RollResult{
		Expression: expression,
		Rolls:      rolls,
		Total:      total,
		Modifier:   modifier,
	}, nil
}

// RollWithModifier выполняет бросок с дополнительным модификатором
func RollWithModifier(expression string, modifier int) (*RollResult, error) {
	result, err := Roll(expression)
	if err != nil {
		return nil, err
	}

	result.Total += modifier
	result.Modifier += modifier

	return result, nil
}

// RollAttack выполняет бросок атаки (d20 + модификатор)
func RollAttack(attackModifier int) *RollResult {
	result, _ := Roll("d20")
	result.Total += attackModifier
	result.Modifier += attackModifier
	return result
}

// RollDamage выполняет бросок урона по заданному выражению
func RollDamage(damageExpression string) (*RollResult, error) {
	return Roll(damageExpression)
}

// CalculateModifier вычисляет модификатор характеристики (по правилам D&D)
// Модификатор = (характеристика - 10) / 2
func CalculateModifier(stat int) int {
	return (stat - 10) / 2
}

type InvalidDiceExpressionError struct {
	Expression string
}

func (e *InvalidDiceExpressionError) Error() string {
	return "invalid dice expression: " + e.Expression
}

// secureRandomInt генерирует криптографически стойкое случайное число от 0 до max-1 (не включительно)
func secureRandomInt(max int) int {
	if max <= 0 {
		return 0
	}

	// Определяем количество байт, необходимых для представления max
	// Используем 64-битное число для максимального диапазона
	var randomValue uint64
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback: если crypto/rand не работает, возвращаем 0
		// На практике это не должно происходить
		return 0
	}
	randomValue = binary.BigEndian.Uint64(b)

	// Используем модульную арифметику для получения значения в диапазоне [0, max)
	// Для избежания bias используем метод отклонения
	// Упрощенная версия: просто используем модуль (для малых max это приемлемо)
	return int(randomValue % uint64(max))
}
