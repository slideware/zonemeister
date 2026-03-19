package netnod

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// validatePathSegment ensures a value is safe to use as a URL path segment.
// It rejects empty strings and strings containing characters that could allow
// path traversal or other injection attacks.
func validatePathSegment(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", name)
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
			return fmt.Errorf("invalid character %q in %s", c, name)
		}
	}
	return nil
}

// Client is an HTTP client for the Netnod Primary DNS API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Netnod API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{},
	}
}

// doRequest performs an authenticated HTTP request. If body is non-nil it is
// JSON-encoded and the Content-Type header is set to application/json.
func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var apiErr struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(resp.Body)
		if jsonErr := json.Unmarshal(raw, &apiErr); jsonErr == nil && apiErr.Error != "" {
			return nil, &APIError{StatusCode: resp.StatusCode, Message: apiErr.Error}
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}

	return resp, nil
}

// decodeJSON decodes the response body as JSON into v and closes the body.
func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ListZones returns a paginated list of zones.
func (c *Client) ListZones(ctx context.Context, offset, limit int) (*ZoneListResponse, error) {
	q := url.Values{}
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(limit))
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/zones?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var result ZoneListResponse
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetZone returns the full detail for a zone.
func (c *Client) GetZone(ctx context.Context, zoneID string) (*Zone, error) {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return nil, err
	}
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/zones/"+zoneID, nil)
	if err != nil {
		return nil, err
	}
	var zone Zone
	if err := decodeJSON(resp, &zone); err != nil {
		return nil, err
	}
	return &zone, nil
}

// CreateZone creates a new zone and returns it.
func (c *Client) CreateZone(ctx context.Context, req CreateZoneRequest) (*Zone, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/zones", req)
	if err != nil {
		return nil, err
	}
	var zone Zone
	if err := decodeJSON(resp, &zone); err != nil {
		return nil, err
	}
	return &zone, nil
}

// UpdateZone replaces the specified fields of a zone (PUT).
func (c *Client) UpdateZone(ctx context.Context, zoneID string, req UpdateZoneRequest) error {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return err
	}
	resp, err := c.doRequest(ctx, http.MethodPut, "/api/v1/zones/"+zoneID, req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// PatchZone performs a partial update on zone RRsets (PATCH).
func (c *Client) PatchZone(ctx context.Context, zoneID string, req PatchZoneRequest) error {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return err
	}
	resp, err := c.doRequest(ctx, http.MethodPatch, "/api/v1/zones/"+zoneID, req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DeleteZone deletes a zone.
func (c *Client) DeleteZone(ctx context.Context, zoneID string) error {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return err
	}
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/zones/"+zoneID, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ExportZone exports the zone in BIND format and returns the raw text.
func (c *Client) ExportZone(ctx context.Context, zoneID string) (string, error) {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return "", err
	}
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/zones/"+zoneID+"/export", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read export body: %w", err)
	}
	return string(data), nil
}

// NotifyZone triggers a DNS NOTIFY for the zone.
func (c *Client) NotifyZone(ctx context.Context, zoneID string) error {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return err
	}
	resp, err := c.doRequest(ctx, http.MethodPut, "/api/v1/zones/"+zoneID+"/notify", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ListDynDNSLabels returns the DynDNS labels for a zone.
func (c *Client) ListDynDNSLabels(ctx context.Context, zoneID string) ([]DynDNSLabel, error) {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return nil, err
	}
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/zones/"+zoneID+"/dyndns", nil)
	if err != nil {
		return nil, err
	}
	var result DynDNSListResponse
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result.Labels, nil
}

// EnableDynDNS enables DynDNS for a label in a zone.
func (c *Client) EnableDynDNS(ctx context.Context, zoneID, label string) (*DynDNSEnableResponse, error) {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return nil, err
	}
	if err := validatePathSegment("label", label); err != nil {
		return nil, err
	}
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/zones/"+zoneID+"/dyndns/"+label, nil)
	if err != nil {
		return nil, err
	}
	var result DynDNSEnableResponse
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DisableDynDNS disables DynDNS for a label in a zone.
func (c *Client) DisableDynDNS(ctx context.Context, zoneID, label string) error {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return err
	}
	if err := validatePathSegment("label", label); err != nil {
		return err
	}
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/zones/"+zoneID+"/dyndns/"+label, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ListACMELabels returns the ACME labels for a zone.
func (c *Client) ListACMELabels(ctx context.Context, zoneID string) ([]ACMELabel, error) {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return nil, err
	}
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/zones/"+zoneID+"/acme", nil)
	if err != nil {
		return nil, err
	}
	var result ACMEListResponse
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result.Labels, nil
}

// EnableACME enables ACME for a label in a zone.
func (c *Client) EnableACME(ctx context.Context, zoneID, label string) (*ACMEEnableResponse, error) {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return nil, err
	}
	if err := validatePathSegment("label", label); err != nil {
		return nil, err
	}
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/zones/"+zoneID+"/acme/"+label, nil)
	if err != nil {
		return nil, err
	}
	var result ACMEEnableResponse
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DisableACME disables ACME for a label in a zone.
func (c *Client) DisableACME(ctx context.Context, zoneID, label string) error {
	if err := validatePathSegment("zone ID", zoneID); err != nil {
		return err
	}
	if err := validatePathSegment("label", label); err != nil {
		return err
	}
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/zones/"+zoneID+"/acme/"+label, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
