package session

import "context"

type DmInput struct {
	Session *GameSession
	Message string
}

type DmResponse struct {
	Text string
}

type DmService interface {
	Respond(ctx context.Context, input DmInput) (DmResponse, error)
}
