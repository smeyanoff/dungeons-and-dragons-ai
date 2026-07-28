package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/pkg/logger"
)

// LocationMover выполняет перемещение партии между связанными локациями мира,
// продвигая игровое время и обновляя текущую позицию на карте.
// Реализуется worldmap.MoveToLocationUseCase.
type LocationMover interface {
	Execute(ctx context.Context, req mapapp.MoveToLocationRequest) (*mapapp.MoveToLocationResponse, error)
}

// MoveToLocationTool позволяет DM явно переместить партию в другую локацию,
// связанную с текущей на карте мира, когда по сюжету игрок(и) отправляются
// или приходят в другое место. В отличие от пассивного анализа текста ответа DM,
// это явный вызов, который синхронно обновляет текущую локацию сессии,
// продвигает игровое время и погоду — то же самое, что происходит при
// навигации по карте через inline-кнопки.
type MoveToLocationTool struct {
	mover     LocationMover
	chatID    int64
	locations []world.Location
}

// NewMoveToLocationTool создает новый инструмент перемещения по карте.
// locations — снимок локаций мира на момент обработки текущего действия,
// используется только для разрешения target_location_name в ID.
func NewMoveToLocationTool(mover LocationMover, chatID int64, locations []world.Location) *MoveToLocationTool {
	return &MoveToLocationTool{
		mover:     mover,
		chatID:    chatID,
		locations: locations,
	}
}

func (t *MoveToLocationTool) Name() string {
	return "move_to_location"
}

func (t *MoveToLocationTool) Description() string {
	return `Перемещает партию в другую локацию, связанную с текущей на карте мира.

Вызывай ЭТОТ инструмент, когда по сюжету игрок(и) отправляются в другую локацию карты
мира — например "иду в лес", "возвращаемся в город", "направляемся к пещере",
"идём на север". НЕ вызывай его для перемещения внутри текущей сцены/комнаты
(например, подойти к двери, отойти в угол) — только для перехода между
локациями графа мира.

Инструмент сам продвинет игровое время (в зависимости от маршрута) и обновит
текущую позицию партии на карте (/map). Если целевая локация не связана с
текущей, инструмент вернёт ошибку — в этом случае объясни в сюжете, что туда
нельзя попасть напрямую отсюда. Если путь заблокирован квестовым предметом,
которого нет у партии, инструмент тоже вернёт ошибку с указанием, какой предмет
нужен — не пытайся обойти это, опиши преграду в сюжете.

Один из параметров обязателен:
- target_location_name: название целевой локации (должна быть известна и связана с текущей)
- direction: направление перемещения (north/south/east/west/up/down или сокращения n/s/e/w/u/d)

Возвращает описание новой локации, прошедшее время в пути и текущую погоду —
используй это в своём ответе игроку.`
}

func (t *MoveToLocationTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"target_location_name": {
			Type:        "string",
			Description: "Название целевой локации, связанной с текущей",
		},
		"direction": {
			Type:        "string",
			Description: "Направление перемещения: north/south/east/west/up/down",
		},
	}
	return BuildJSONSchema(properties, nil)
}

func (t *MoveToLocationTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	targetName := strings.TrimSpace(getStringParam(args, "target_location_name"))
	direction := strings.TrimSpace(getStringParam(args, "direction"))

	req := mapapp.MoveToLocationRequest{ChatID: t.chatID}

	switch {
	case targetName != "":
		id, found := t.resolveLocationID(targetName)
		if !found {
			return map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("локация %q не найдена среди известных локаций мира", targetName),
			}, nil
		}
		req.ToLocationID = &id
	case direction != "":
		req.Direction = direction
	default:
		return nil, fmt.Errorf("either target_location_name or direction is required")
	}

	resp, err := t.mover.Execute(ctx, req)
	if err != nil {
		logger.Warn("MoveToLocationTool: move failed",
			logger.Int64("chat_id", t.chatID),
			logger.ErrorField(err),
		)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	logger.Info("MoveToLocationTool: moved",
		logger.Int64("chat_id", t.chatID),
		logger.String("from", resp.From.Name),
		logger.String("to", resp.To.Name),
	)

	return map[string]interface{}{
		"success": true,
		"from":    resp.From.Name,
		"to":      resp.To.Name,
		"message": resp.Message,
	}, nil
}

// resolveLocationID ищет локацию по имени (без учета регистра) среди
// известных локаций мира: сперва точное совпадение, затем частичное.
func (t *MoveToLocationTool) resolveLocationID(name string) (uint, bool) {
	nameLower := strings.ToLower(name)

	for _, loc := range t.locations {
		if strings.ToLower(loc.Name) == nameLower {
			return loc.ID, true
		}
	}
	for _, loc := range t.locations {
		locNameLower := strings.ToLower(loc.Name)
		if strings.Contains(locNameLower, nameLower) || strings.Contains(nameLower, locNameLower) {
			return loc.ID, true
		}
	}

	return 0, false
}
