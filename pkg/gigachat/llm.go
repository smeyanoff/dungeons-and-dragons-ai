package gigachat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

func (c *Client) Chat(ctx context.Context, model string, message string) (*ChatResponse, error) {
	reqBody := ChatRequest{
		Model:   model,
		Message: message,
	}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/chat/completions", c.cfg.APIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gigachat error status %d: %s", resp.StatusCode, string(data))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}
	return &chatResp, nil
}
