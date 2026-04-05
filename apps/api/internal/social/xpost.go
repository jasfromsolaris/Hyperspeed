package social

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

// PostTweetV2 posts a tweet using X API v2 OAuth2 Bearer token (user or app with tweet.write).
func PostTweetV2(ctx context.Context, bearerToken, text string) (tweetID string, err error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("empty tweet text")
	}
	if len(text) > 280 {
		return "", fmt.Errorf("tweet text exceeds 280 characters")
	}
	body := map[string]string{"text": text}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitter.com/2/tweets", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("x api %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.Data.ID, nil
}
