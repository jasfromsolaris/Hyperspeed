package provisioning

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ClaimError is returned when the control plane rejects a claim or the request fails.
type ClaimError struct {
	HTTPStatus int
	Code       string
}

func (e *ClaimError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

// PostClaims POSTs {slug, ipv4} to the Hyperspeed provisioning gateway at {gatewayBaseURL}/v1/claims
// using install HMAC headers (no control-plane bearer on the self-hosted API).
func PostClaims(ctx context.Context, gatewayBaseURL, installID, installSecret string, httpClient *http.Client, slug, ipv4 string) (statusCode int, respBody []byte, err error) {
	payload, err := json.Marshal(map[string]string{
		"slug": slug,
		"ipv4": ipv4,
	})
	if err != nil {
		return 0, nil, err
	}
	base := strings.TrimRight(strings.TrimSpace(gatewayBaseURL), "/")
	path := "/v1/claims"
	ts := time.Now().Unix()
	sig := SignGatewayRequest(installSecret, ts, http.MethodPost, path, payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderInstallID, strings.TrimSpace(installID))
	req.Header.Set(HeaderTimestamp, formatUnix(ts))
	req.Header.Set(HeaderSignature, sig)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// DeleteGatewayClaim DELETEs /v1/claims/{slug} on the provisioning gateway (signed).
func DeleteGatewayClaim(ctx context.Context, gatewayBaseURL, installID, installSecret string, httpClient *http.Client, slug string) (statusCode int, respBody []byte, err error) {
	base := strings.TrimRight(strings.TrimSpace(gatewayBaseURL), "/")
	path := "/v1/claims/" + url.PathEscape(strings.TrimSpace(slug))
	ts := time.Now().Unix()
	sig := SignGatewayRequest(installSecret, ts, http.MethodDelete, path, nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set(HeaderInstallID, strings.TrimSpace(installID))
	req.Header.Set(HeaderTimestamp, formatUnix(ts))
	req.Header.Set(HeaderSignature, sig)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func formatUnix(ts int64) string {
	return strconv.FormatInt(ts, 10)
}

// MapClaimFailure maps a non-success control-plane response to an API status and error code.
func MapClaimFailure(statusCode int, respBody []byte) (clientStatus int, clientCode string) {
	var errObj map[string]any
	if json.Unmarshal(respBody, &errObj) == nil {
		if e, ok := errObj["error"].(string); ok {
			switch e {
			case "invalid_slug":
				return http.StatusBadRequest, "invalid_slug"
			case "invalid_ipv4":
				return http.StatusBadRequest, "invalid_ipv4"
			case "slug_taken":
				return http.StatusConflict, "slug_taken"
			case "rate_limited":
				return http.StatusTooManyRequests, "rate_limited"
			case "invalid_install", "gateway_misconfigured":
				return http.StatusBadGateway, "provisioning_unavailable"
			}
		}
	}
	if statusCode == http.StatusUnauthorized {
		return http.StatusBadGateway, "provisioning_unavailable"
	}
	return http.StatusBadGateway, "provisioning_unavailable"
}

// ErrFromClaimResponse returns nil on 2xx, or *ClaimError for failure responses.
func ErrFromClaimResponse(statusCode int, respBody []byte, networkErr error) error {
	if networkErr != nil {
		return &ClaimError{HTTPStatus: http.StatusBadGateway, Code: "provisioning_unavailable"}
	}
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	st, code := MapClaimFailure(statusCode, respBody)
	return &ClaimError{HTTPStatus: st, Code: code}
}

// ErrProvisioningUnavailable is used when the server is not configured to call the control plane.
func ErrProvisioningUnavailable() error {
	return &ClaimError{HTTPStatus: http.StatusServiceUnavailable, Code: "provisioning_unavailable"}
}
