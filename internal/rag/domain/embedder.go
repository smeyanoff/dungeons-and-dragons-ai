package domain

type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
}

