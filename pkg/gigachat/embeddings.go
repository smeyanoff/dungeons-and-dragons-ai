package gigachat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultEmbeddingModel = "GigaChat-Embeddings"

func (c *Client) Embed(
	ctx context.Context,
	text string,
) ([]float32, error) {
	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (c *Client) EmbedBatch(
	ctx context.Context,
	texts []string,
) ([][]float32, error) {

	reqBody := EmbeddingRequest{
		Model: defaultEmbeddingModel,
		Input: texts,
	}

	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/embeddings", c.cfg.APIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gigachat embeddings error %d: %s", resp.StatusCode, string(data))
	}

	var embResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, err
	}

	result := make([][]float32, len(embResp.Data))
	for _, d := range embResp.Data {
		result[d.Index] = d.Embedding
	}

	return result, nil
}
