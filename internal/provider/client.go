// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal HTTP client for the Certificate Checker REST API (/api/v1).
// The API is deliberately small: domains are immutable apart from their uptime flag,
// so the only update call is the PATCH that toggles it.
type Client struct {
	endpoint   string
	token      string
	userAgent  string
	httpClient *http.Client
}

// Domain mirrors the public API contract of a domain. The id is a uuid string.
// Level and the timestamps are pointers because they are null until the first
// successful certificate check. UptimeEnabled is a pointer as well, so that a
// response without the field (an older API) reads as "unspecified" rather than
// as false — the server-side default is on.
type Domain struct {
	ID                  string  `json:"id"`
	Hostname            string  `json:"hostname"`
	DaysUntilExpiration *int64  `json:"days_until_expiration"`
	Level               *string `json:"level"`
	UptimeEnabled       *bool   `json:"uptime_enabled"`
	LastCheckedAt       *string `json:"last_checked_at"`
	CreatedAt           *string `json:"created_at"`
}

/*
Certificate mirrors the public API contract of the certificate behind a domain's day
count: the one the most recent successful check saw, or, for a hostname that serves a
different certificate per server, the one of those that expires first. Every scalar is a
pointer because a check whose stored payload can no longer be read answers with every
field null rather than with an error.
*/
type Certificate struct {
	Subject            *string  `json:"subject"`
	Issuer             *string  `json:"issuer"`
	IssuerOrganization *string  `json:"issuer_organization"`
	SerialNumber       *string  `json:"serial_number"`
	ValidFrom          *string  `json:"valid_from"`
	ValidTo            *string  `json:"valid_to"`
	Names              []string `json:"names"`
	FingerprintSHA256  *string  `json:"fingerprint_sha256"`
	FingerprintSHA1    *string  `json:"fingerprint_sha1"`
	SignatureAlgorithm *string  `json:"signature_algorithm"`
	KeyType            *string  `json:"key_type"`
	KeyBits            *int64   `json:"key_bits"`
	RemoteAddress      *string  `json:"remote_address"`
}

/*
Uptime mirrors the public API contract of a domain's uptime summary: what the Uptime section
of the domain page shows. The summary always exists, also before the first ping — Status is
then "unknown" and the measurements are null, which is why those are pointers. ResponseTime is
a pointer as a whole: the API sends null until a ping in the current period got a response.
*/
type Uptime struct {
	Enabled       bool                `json:"enabled"`
	Paused        bool                `json:"paused"`
	Status        string              `json:"status"`
	Since         *string             `json:"since"`
	LastCheckedAt *string             `json:"last_checked_at"`
	Percentages   UptimePercentages   `json:"uptime"`
	ResponseTime  *UptimeResponseTime `json:"response_time"`
}

// UptimePercentages holds the observed uptime over the two windows the dashboard shows.
type UptimePercentages struct {
	Last30Days *float64 `json:"30d"`
	Last90Days *float64 `json:"90d"`
}

// UptimeResponseTime holds the response times of the current period, in milliseconds.
type UptimeResponseTime struct {
	AverageMs *int64 `json:"average_ms"`
	MinMs     *int64 `json:"min_ms"`
	MaxMs     *int64 `json:"max_ms"`
}

// APIError carries the HTTP status, the API's message and — where the API sends one —
// its stable error code, so callers can translate specific refusals (403 plan gate,
// 403 account-level, 404 gone, 429 rate limit, 503 kill switch) into actionable
// diagnostics. The code is empty for refusals that carry only a message.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API returned HTTP %d: %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether the error is the API's 404 — which the API also returns
// for domains owned by someone else, indistinguishable from non-existent ones.
func IsNotFound(err error) bool {
	apiErr, ok := err.(*APIError)

	return ok && apiErr.StatusCode == http.StatusNotFound
}

func NewClient(endpoint, token, version string) *Client {
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		token:      token,
		userAgent:  "terraform-provider-certificatechecker/" + version,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetDomain fetches one domain by id.
func (c *Client) GetDomain(ctx context.Context, id string) (*Domain, error) {
	var envelope struct {
		Data Domain `json:"data"`
	}

	if err := c.do(ctx, http.MethodGet, "/api/v1/domains/"+id, nil, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// GetCertificate fetches the current certificate of a domain. The API answers 404 both for
// a domain that does not exist (or is not ours) and for one that has no successful check
// yet; callers that need to tell the two apart look the domain up first.
func (c *Client) GetCertificate(ctx context.Context, domainID string) (*Certificate, error) {
	var envelope struct {
		Data Certificate `json:"data"`
	}

	if err := c.do(ctx, http.MethodGet, "/api/v1/domains/"+domainID+"/certificate", nil, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// GetUptime fetches the uptime summary of a domain. Unlike the certificate, the summary exists
// from the moment the domain does, so a 404 here can only mean an unknown domain.
func (c *Client) GetUptime(ctx context.Context, domainID string) (*Uptime, error) {
	var envelope struct {
		Data Uptime `json:"data"`
	}

	if err := c.do(ctx, http.MethodGet, "/api/v1/domains/"+domainID+"/uptime", nil, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// FindDomainByHostname walks the paginated list until it finds an exact hostname
// match. Nil without error means no domain with that hostname exists.
func (c *Client) FindDomainByHostname(ctx context.Context, hostname string) (*Domain, error) {
	for page := 1; ; page++ {
		var envelope struct {
			Data  []Domain `json:"data"`
			Links struct {
				Next *string `json:"next"`
			} `json:"links"`
		}

		path := fmt.Sprintf("/api/v1/domains?per_page=100&page=%d", page)
		if err := c.do(ctx, http.MethodGet, path, nil, &envelope); err != nil {
			return nil, err
		}

		for i := range envelope.Data {
			if envelope.Data[i].Hostname == hostname {
				return &envelope.Data[i], nil
			}
		}

		if envelope.Links.Next == nil {
			return nil, nil
		}
	}
}

// CreateDomain registers a hostname; the first certificate check is queued
// server-side, so the returned expiry fields start out null. A nil uptimeEnabled
// leaves the field out of the request, which lets the API apply its own default.
func (c *Client) CreateDomain(ctx context.Context, hostname string, uptimeEnabled *bool) (*Domain, error) {
	var envelope struct {
		Data Domain `json:"data"`
	}

	body := map[string]any{"hostname": hostname}
	if uptimeEnabled != nil {
		body["uptime_enabled"] = *uptimeEnabled
	}

	if err := c.do(ctx, http.MethodPost, "/api/v1/domains", body, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// UpdateDomain switches uptime checks on or off. It is the only mutable part of a
// domain: the API's PATCH accepts this one field and refuses anything else.
func (c *Client) UpdateDomain(ctx context.Context, id string, uptimeEnabled bool) (*Domain, error) {
	var envelope struct {
		Data Domain `json:"data"`
	}

	body := map[string]any{"uptime_enabled": uptimeEnabled}
	if err := c.do(ctx, http.MethodPatch, "/api/v1/domains/"+id, body, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// DeleteDomain removes a domain by id.
func (c *Client) DeleteDomain(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/domains/"+id, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return newAPIError(resp.StatusCode, resp.Body)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response of %s %s: %w", method, path, err)
		}
	}

	return nil
}

// newAPIError extracts Laravel's {"message": "..."} error body, plus the stable
// "error" code that account-level refusals carry next to it, falling back to the
// raw body so unexpected responses still surface something readable.
func newAPIError(statusCode int, body io.Reader) *APIError {
	raw, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil {
		return &APIError{StatusCode: statusCode, Message: "unreadable response body"}
	}

	var envelope struct {
		Code    string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Message != "" {
		return &APIError{StatusCode: statusCode, Code: envelope.Code, Message: envelope.Message}
	}

	return &APIError{StatusCode: statusCode, Message: strings.TrimSpace(string(raw))}
}
