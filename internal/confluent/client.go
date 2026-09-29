// Package confluent is a minimal REST client for the Confluent Cloud IAM v2 API
// covering identity pools (authN) and role bindings (authZ). It intentionally
// avoids the full ccloud-sdk-go-v2 dependency: the operator needs only a small,
// well-understood slice of the API for the PoC.
package confluent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the Confluent Cloud REST API using an API key/secret pair via
// HTTP Basic auth.
type Client struct {
	baseURL    string
	apiKey     string
	apiSecret  string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client (useful for tests).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient builds a Confluent Cloud REST client. baseURL defaults to
// https://api.confluent.cloud when empty.
func NewClient(baseURL, apiKey, apiSecret string, opts ...Option) *Client {
	if baseURL == "" {
		baseURL = "https://api.confluent.cloud"
	}
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// APIError is returned for non-2xx responses.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("confluent api error: status=%d body=%s", e.StatusCode, e.Body)
}

// IsNotFound reports whether err is an APIError with HTTP 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if e, ok := err.(*APIError); ok {
		apiErr = e
	}
	return apiErr != nil && apiErr.StatusCode == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, c.apiSecret)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Identity pools (authentication)
// ---------------------------------------------------------------------------

// IdentityPool is a Confluent Cloud identity pool.
type IdentityPool struct {
	ID            string `json:"id,omitempty"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description,omitempty"`
	IdentityClaim string `json:"identity_claim"`
	Filter        string `json:"filter"`
}

type identityPoolList struct {
	Data     []IdentityPool `json:"data"`
	Metadata struct {
		Next string `json:"next"`
	} `json:"metadata"`
}

func poolsPath(providerID string) string {
	return fmt.Sprintf("/iam/v2/identity-providers/%s/identity-pools", url.PathEscape(providerID))
}

// CreateIdentityPool creates a pool under the given identity provider.
func (c *Client) CreateIdentityPool(ctx context.Context, providerID string, pool IdentityPool) (*IdentityPool, error) {
	var out IdentityPool
	if err := c.do(ctx, http.MethodPost, poolsPath(providerID), nil, pool, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetIdentityPool fetches a pool by id.
func (c *Client) GetIdentityPool(ctx context.Context, providerID, poolID string) (*IdentityPool, error) {
	var out IdentityPool
	p := poolsPath(providerID) + "/" + url.PathEscape(poolID)
	if err := c.do(ctx, http.MethodGet, p, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindIdentityPoolByDisplayName lists pools under the provider and returns the
// first whose display name matches. Returns nil (no error) when none match.
func (c *Client) FindIdentityPoolByDisplayName(ctx context.Context, providerID, displayName string) (*IdentityPool, error) {
	path := poolsPath(providerID)
	query := url.Values{"page_size": []string{"100"}}
	for {
		var page identityPoolList
		if err := c.do(ctx, http.MethodGet, path, query, nil, &page); err != nil {
			return nil, err
		}
		for i := range page.Data {
			if page.Data[i].DisplayName == displayName {
				return &page.Data[i], nil
			}
		}
		if page.Metadata.Next == "" {
			return nil, nil
		}
		next, err := nextPageToken(page.Metadata.Next)
		if err != nil {
			return nil, err
		}
		query.Set("page_token", next)
	}
}

// UpdateIdentityPool patches a pool's mutable fields (filter/description).
func (c *Client) UpdateIdentityPool(ctx context.Context, providerID, poolID string, pool IdentityPool) (*IdentityPool, error) {
	var out IdentityPool
	p := poolsPath(providerID) + "/" + url.PathEscape(poolID)
	if err := c.do(ctx, http.MethodPatch, p, nil, pool, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteIdentityPool deletes a pool by id. A 404 is treated as success.
func (c *Client) DeleteIdentityPool(ctx context.Context, providerID, poolID string) error {
	p := poolsPath(providerID) + "/" + url.PathEscape(poolID)
	err := c.do(ctx, http.MethodDelete, p, nil, nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Role bindings (authorization)
// ---------------------------------------------------------------------------

// RoleBinding is a Confluent Cloud role binding.
type RoleBinding struct {
	ID         string `json:"id,omitempty"`
	Principal  string `json:"principal"`
	RoleName   string `json:"role_name"`
	CRNPattern string `json:"crn_pattern"`
}

type roleBindingList struct {
	Data     []RoleBinding `json:"data"`
	Metadata struct {
		Next string `json:"next"`
	} `json:"metadata"`
}

const roleBindingsPath = "/iam/v2/role-bindings"

// CreateRoleBinding creates a role binding.
func (c *Client) CreateRoleBinding(ctx context.Context, rb RoleBinding) (*RoleBinding, error) {
	var out RoleBinding
	if err := c.do(ctx, http.MethodPost, roleBindingsPath, nil, rb, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRoleBinding deletes a role binding by id. A 404 is treated as success.
func (c *Client) DeleteRoleBinding(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, roleBindingsPath+"/"+url.PathEscape(id), nil, nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

// ListRoleBindingsForPrincipal returns all role bindings for a principal within
// a CRN scope (the org/environment CRN). crnScope may be empty.
func (c *Client) ListRoleBindingsForPrincipal(ctx context.Context, principal, crnScope string) ([]RoleBinding, error) {
	query := url.Values{
		"principal": []string{principal},
		"page_size": []string{"100"},
	}
	if crnScope != "" {
		query.Set("crn_pattern", crnScope)
	}
	var all []RoleBinding
	for {
		var page roleBindingList
		if err := c.do(ctx, http.MethodGet, roleBindingsPath, query, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if page.Metadata.Next == "" {
			return all, nil
		}
		next, err := nextPageToken(page.Metadata.Next)
		if err != nil {
			return nil, err
		}
		query.Set("page_token", next)
	}
}

// nextPageToken extracts the page_token query param from a Confluent pagination
// "next" URL.
func nextPageToken(nextURL string) (string, error) {
	u, err := url.Parse(nextURL)
	if err != nil {
		return "", fmt.Errorf("parse next url: %w", err)
	}
	return u.Query().Get("page_token"), nil
}
