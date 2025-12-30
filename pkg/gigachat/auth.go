package gigachat

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strings"
	"sync"
)

type TokenResponse struct {
    AccessToken string `json:"access_token"`
    ExpiresAt   int64  `json:"expires_at"` // unix timestamp
}

type authClient struct {
    cfg   Config
    token *TokenResponse
	mu    sync.Mutex
}

func newAuthClient(cfg Config) *authClient {
    return &authClient{cfg: cfg}
}

// getToken получает новый токен и кэширует его.
func (a *authClient) getToken(ctx context.Context) (string, error) {
    a.mu.Lock()
    defer a.mu.Unlock()

    if a.token != nil {
        // TODO: можно проверять expires_at
        return a.token.AccessToken, nil
    }

    authHeader := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", a.cfg.ClientID, a.cfg.ClientSecret)))

    form := url.Values{}
    form.Set("scope", a.cfg.Scope)

    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        fmt.Sprintf("%s/api/v2/oauth", a.cfg.AuthBaseURL),
        strings.NewReader(form.Encode()),
    )
    if err != nil {
        return "", err
    }

    req.Header.Set("Authorization", "Basic "+authHeader)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Accept", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return "", fmt.Errorf("gigachat auth error: %s", resp.Status)
    }

    var token TokenResponse
    if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
        return "", err
    }

    a.token = &token
    return token.AccessToken, nil
}
