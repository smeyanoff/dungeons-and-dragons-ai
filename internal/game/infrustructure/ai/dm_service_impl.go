package ai

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/rag/application"
)

type DmService struct {
	rag rag.RetrieveContext
	llm LLMClient
}

func NewDmService(
	rag rag.RetrieveContext,
	llm LLMClient,
) *DmService {
	return &DmService{
		rag: rag,
		llm: llm,
	}
}

func (s *DmService) Respond(
	ctx context.Context,
	input session.DmInput,
) (session.DmResponse, error) {

	contextDocs, err := s.rag.Execute(ctx, input.Message)
	if err != nil {
		return session.DmResponse{}, err
	}

	prompt := buildPrompt(input.Session, contextDocs, input.Message)

	text, err := s.llm.Generate(ctx, prompt)
	if err != nil {
		return session.DmResponse{}, err
	}

	return session.DmResponse{Text: text}, nil
}

func buildPrompt(
	session *session.GameSession,
	docs []string,
	playerMessage string,
) string {

	return fmt.Sprintf(`
You are a Dungeon Master.

World:
%s

Recent events:
%s

Player says:
"%s"

Respond as DM:
`,
		summarizeWorld(session.World),
		summarizeEvents(session),
		playerMessage,
	)
}
