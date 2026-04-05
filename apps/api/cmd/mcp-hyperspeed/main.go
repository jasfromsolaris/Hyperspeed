// Command mcp-hyperspeed is a stdio MCP server that forwards tool calls to the Hyperspeed API.
//
// Environment:
//   - HYPERSPEED_API_URL — base URL, e.g. http://localhost:8080
//   - HYPERSPEED_TOKEN — service account token (sa_…)
//   - HYPERSPEED_ORG_ID — organization UUID
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	base := strings.TrimRight(os.Getenv("HYPERSPEED_API_URL"), "/")
	token := strings.TrimSpace(os.Getenv("HYPERSPEED_TOKEN"))
	orgID := strings.TrimSpace(os.Getenv("HYPERSPEED_ORG_ID"))
	if base == "" || token == "" || orgID == "" {
		fmt.Fprintf(os.Stderr, "mcp-hyperspeed: missing required env vars.\n")
		fmt.Fprintf(os.Stderr, "  HYPERSPEED_API_URL (e.g. http://localhost:8080)\n")
		fmt.Fprintf(os.Stderr, "  HYPERSPEED_TOKEN (service token: sa_...)\n")
		fmt.Fprintf(os.Stderr, "  HYPERSPEED_ORG_ID (organization UUID)\n")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	sc := bufio.NewScanner(os.Stdin)
	// Large lines for JSON payloads.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Method == "" {
			continue
		}
		if strings.HasPrefix(msg.Method, "notifications/") {
			continue
		}
		if msg.ID == nil {
			continue
		}

		switch msg.Method {
		case "initialize":
			writeJSON(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    "hyperspeed-mcp",
						"version": "0.1.0",
					},
				},
			})
		case "tools/list":
			tools, err := fetchTools(client, base, token, orgID)
			if err != nil {
				writeErr(os.Stdout, msg.ID, -32000, err.Error())
				continue
			}
			writeJSON(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result":  map[string]any{"tools": tools},
			})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(msg.Params, &p); err != nil {
				writeErr(os.Stdout, msg.ID, -32602, "invalid params")
				continue
			}
			res, err := invokeTool(client, base, token, orgID, p.Name, p.Arguments)
			if err != nil {
				writeErr(os.Stdout, msg.ID, -32000, err.Error())
				continue
			}
			// MCP tools/call result shape.
			content := []map[string]any{
				{"type": "text", "text": string(res)},
			}
			writeJSON(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result": map[string]any{
					"content": content,
				},
			})
		default:
			writeErr(os.Stdout, msg.ID, -32601, "method not found")
		}
	}
}

func writeJSON(w io.Writer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = w.Write(append(b, '\n'))
}

func writeErr(w io.Writer, id json.RawMessage, code int, message string) {
	writeJSON(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func fetchTools(client *http.Client, base, token, orgID string) ([]any, error) {
	req, err := http.NewRequest(http.MethodGet, base+"/api/v1/organizations/"+orgID+"/agent-tools/tools", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := doWithRetry(client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("api %s: auth failed (check HYPERSPEED_TOKEN and org membership)", resp.Status)
		}
		return nil, fmt.Errorf("api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Tools []any `json:"tools"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

func invokeTool(client *http.Client, base, token, orgID, tool string, args json.RawMessage) ([]byte, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	mode, sessionID, strippedArgs := extractInvokeMeta(args)
	payload := struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
		Mode      string          `json:"mode,omitempty"`
		SessionID string          `json:"session_id,omitempty"`
	}{
		Tool:      tool,
		Arguments: strippedArgs,
		Mode:      mode,
		SessionID: sessionID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/organizations/"+orgID+"/agent-tools/invoke", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := doWithRetry(client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("api %s: auth failed (check HYPERSPEED_TOKEN and org membership)", resp.Status)
		}
		return nil, fmt.Errorf("api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func doWithRetry(client *http.Client, req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusTooManyRequests && (resp.StatusCode < 500 || resp.StatusCode > 599) {
		return resp, nil
	}
	// Retry once for transient backend failures.
	resp.Body.Close()
	time.Sleep(300 * time.Millisecond)
	req2 := req.Clone(req.Context())
	if req.GetBody != nil {
		b, err := req.GetBody()
		if err == nil {
			req2.Body = b
		}
	}
	return client.Do(req2)
}

// extractInvokeMeta reads optional MCP-side control metadata from arguments:
// { "_hyperspeed": { "mode": "ask|plan|agent", "session_id": "..." }, ...toolArgs }
func extractInvokeMeta(args json.RawMessage) (mode string, sessionID string, outArgs json.RawMessage) {
	outArgs = args
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return "", "", outArgs
	}
	rawMeta, ok := m["_hyperspeed"]
	if !ok {
		return "", "", outArgs
	}
	meta, ok := rawMeta.(map[string]any)
	if !ok {
		return "", "", outArgs
	}
	if v, ok := meta["mode"].(string); ok {
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "ask" || v == "plan" || v == "agent" {
			mode = v
		}
	}
	if v, ok := meta["session_id"].(string); ok {
		sessionID = strings.TrimSpace(v)
	}
	delete(m, "_hyperspeed")
	if b, err := json.Marshal(m); err == nil {
		outArgs = b
	}
	return mode, sessionID, outArgs
}
