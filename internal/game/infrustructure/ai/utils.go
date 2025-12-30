package ai

import (
    "fmt"
    "strings"

    "dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// SummarizeEvents собирает последние события истории для DM
func SummarizeEvents(events []event.StoryEvent, max int) string {
    if len(events) == 0 {
        return "No recent events."
    }

    start := 0
    if len(events) > max {
        start = len(events) - max
    }

    sb := strings.Builder{}
    for _, e := range events[start:] {
        sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", e.CreatedAt.Format("15:04"), e.AuthorName, e.Content))
    }

    return sb.String()
}

// summarizeWorld собирает текстовое описание мира для DM
func SummarizeWorld(w world.World) string {
    sb := strings.Builder{}

    sb.WriteString(fmt.Sprintf("World: %s\n", w.Name))
    sb.WriteString(fmt.Sprintf("Description: %s\n", w.Description))

    sb.WriteString("Locations:\n")
    for _, loc := range w.Locations {
        sb.WriteString(fmt.Sprintf("- %s: %s\n", loc.Name, loc.Description))

        if len(loc.NPCs) > 0 {
            sb.WriteString("  NPCs: ")
            npcs := make([]string, 0, len(loc.NPCs))
            for _, npc := range loc.NPCs {
                npcs = append(npcs, fmt.Sprintf("%s (%s)", npc.Name, npc.Role))
            }
            sb.WriteString(strings.Join(npcs, ", "))
            sb.WriteString("\n")
        }

        if len(loc.Monsters) > 0 {
            sb.WriteString("  Monsters: ")
            monsters := make([]string, 0, len(loc.Monsters))
            for _, m := range loc.Monsters {
                monsters = append(monsters, fmt.Sprintf("%s (HP:%d)", m.Name, m.HP))
            }
            sb.WriteString(strings.Join(monsters, ", "))
            sb.WriteString("\n")
        }

        if len(loc.Items) > 0 {
            sb.WriteString("  Items: ")
            items := make([]string, 0, len(loc.Items))
            for _, it := range loc.Items {
                items = append(items, fmt.Sprintf("%s (%s)", it.Name, it.Type))
            }
            sb.WriteString(strings.Join(items, ", "))
            sb.WriteString("\n")
        }
    }

    return sb.String()
}
