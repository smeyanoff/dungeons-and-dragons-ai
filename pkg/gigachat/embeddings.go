package gigachat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const defaultEmbeddingModel = string(ModelEmbeddings)

// embeddingModel возвращает модель эмбеддингов из Config клиента, либо значение по умолчанию,
// если Config.EmbeddingModel не задан. Доступные модели см. в models.go (KnownEmbeddingModels).
func (c *Client) embeddingModel() string {
	if c.cfg.EmbeddingModel != "" {
		return c.cfg.EmbeddingModel
	}
	return defaultEmbeddingModel
}

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
		Model: c.embeddingModel(),
		Input: texts,
	}

	body, _ := json.Marshal(reqBody)

	apiURL := c.cfg.APIBaseURL
	// Fix incorrect URL if it points to auth endpoint
	if strings.Contains(apiURL, ":9443") && !strings.Contains(apiURL, "/api/") {
		apiURL = strings.Replace(apiURL, ":9443", ":9443/api/v1", 1)
		log.Printf("[GigaChat] Fixed API URL from auth endpoint to API endpoint: %s", apiURL)
	}

	url := fmt.Sprintf("%s/embeddings", apiURL)

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
