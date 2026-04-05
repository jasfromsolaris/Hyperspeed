package cf

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

const apiBase = "https://api.cloudflare.com/client/v4"

type Client struct {
	Token  string
	ZoneID string
	HTTP   *http.Client
}

type cfResponse struct {
	Success bool            `json:"success"`
	Errors  []cfError       `json:"errors"`
	Result  json.RawMessage `json:"result"`
	ResultS []dnsRecord     `json:"result"` // list returns array
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type dnsRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// UpsertA creates or updates an A record. recordName must be zone-relative (e.g. www.acme for zone
// example.com). fqdn is the full hostname (e.g. www.acme.example.com) used to find existing rows
// that may have been created with either relative or legacy full-name forms.
func (c *Client) UpsertA(ctx context.Context, recordName, fqdn, ipv4 string, proxied bool) (recordID string, err error) {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	existing, err := c.listAFirstMatch(ctx, recordName, fqdn)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"type":    "A",
		"name":    recordName,
		"content": ipv4,
		"ttl":     300,
		"proxied": proxied,
	}
	if len(existing) > 0 {
		id := existing[0].ID
		if err := c.patchRecord(ctx, id, body); err != nil {
			return "", err
		}
		return id, nil
	}
	return c.createRecord(ctx, body)
}

// listAFirstMatch tries zone-relative name first (Cloudflare's preferred form), then full FQDN.
func (c *Client) listAFirstMatch(ctx context.Context, recordName, fqdn string) ([]dnsRecord, error) {
	rows, err := c.listA(ctx, recordName)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	return c.listA(ctx, fqdn)
}

func (c *Client) DeleteRecord(ctx context.Context, recordID string) error {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/zones/%s/dns_records/%s", apiBase, c.ZoneID, recordID), nil)
	if err != nil {
		return err
	}
	c.setAuth(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare delete: %s: %s", resp.Status, string(b))
	}
	var out cfResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("cloudflare: %s", summarizeErrors(out.Errors))
	}
	return nil
}

func (c *Client) listA(ctx context.Context, fqdn string) ([]dnsRecord, error) {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	u := fmt.Sprintf("%s/zones/%s/dns_records?type=A&name=%s", apiBase, c.ZoneID, url.QueryEscape(fqdn))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cloudflare list: %s: %s", resp.Status, string(b))
	}
	var out struct {
		Success bool        `json:"success"`
		Errors  []cfError   `json:"errors"`
		Result  []dnsRecord `json:"result"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("cloudflare: %s", summarizeErrors(out.Errors))
	}
	return out.Result, nil
}

func (c *Client) createRecord(ctx context.Context, body map[string]any) (string, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/zones/%s/dns_records", apiBase, c.ZoneID), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("cloudflare create: %s: %s", resp.Status, string(b))
	}
	var out struct {
		Success bool      `json:"success"`
		Errors  []cfError `json:"errors"`
		Result  dnsRecord `json:"result"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if !out.Success {
		return "", fmt.Errorf("cloudflare: %s", summarizeErrors(out.Errors))
	}
	return out.Result.ID, nil
}

func (c *Client) patchRecord(ctx context.Context, id string, body map[string]any) error {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		fmt.Sprintf("%s/zones/%s/dns_records/%s", apiBase, c.ZoneID, id), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare patch: %s: %s", resp.Status, string(b))
	}
	var out cfResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("cloudflare: %s", summarizeErrors(out.Errors))
	}
	return nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
}

// DNSRecord exposes record id for delete fallbacks.
type DNSRecord struct {
	ID string
}

// ListARecords lists A records for an exact Cloudflare list "name" filter (zone-relative or FQDN).
func (c *Client) ListARecords(ctx context.Context, fqdn string) ([]DNSRecord, error) {
	rows, err := c.listA(ctx, fqdn)
	if err != nil {
		return nil, err
	}
	out := make([]DNSRecord, len(rows))
	for i, r := range rows {
		out[i] = DNSRecord{ID: r.ID}
	}
	return out, nil
}

// ListARecordsAny returns A records for the first matching name filter, in order.
func (c *Client) ListARecordsAny(ctx context.Context, names ...string) ([]DNSRecord, error) {
	for _, n := range names {
		if n == "" {
			continue
		}
		rows, err := c.listA(ctx, n)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			out := make([]DNSRecord, len(rows))
			for i, r := range rows {
				out[i] = DNSRecord{ID: r.ID}
			}
			return out, nil
		}
	}
	return nil, nil
}

func summarizeErrors(errs []cfError) string {
	if len(errs) == 0 {
		return "unknown error"
	}
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Message)
	}
	return b.String()
}
