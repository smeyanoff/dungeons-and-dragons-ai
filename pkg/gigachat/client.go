package gigachat

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "bytes"
)

type Client struct {
    auth *authClient
    cfg  Config
}

func NewClient(cfg Config) *Client {
    return &Client{
        auth: newAuthClient(cfg),
        cfg:  cfg,
    }
}

func (c *Client) doRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
    token, err := c.auth.getToken(ctx)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/json")
    req.Header.Set("Content-Type", "application/json")

    return http.DefaultClient.Do(req)
}
