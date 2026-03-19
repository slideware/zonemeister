package netnod

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// TSIGKey represents a TSIG key returned by the NDS API.
type TSIGKey struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Alg  string `json:"alg"`
}

// TSIGClient is an HTTP client for the Netnod NDS API (TSIG endpoint).
type TSIGClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewTSIGClient creates a new TSIG API client.
func NewTSIGClient(baseURL, token string) *TSIGClient {
	return &TSIGClient{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{},
	}
}

// ListKeys returns all available TSIG keys.
func (c *TSIGClient) ListKeys(ctx context.Context) ([]TSIGKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/apiv3/tsig/", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("TSIG API error: %s", string(body)),
		}
	}

	var keys []TSIGKey
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return keys, nil
}
