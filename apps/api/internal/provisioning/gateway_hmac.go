package provisioning

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Headers sent to the public provisioning gateway.
const (
	HeaderInstallID = "X-Hyperspeed-Install-Id"
	HeaderTimestamp = "X-Hyperspeed-Timestamp"
	HeaderSignature = "X-Hyperspeed-Signature"
)

// CanonicalSignPayload builds the string that is HMAC-SHA256 signed for gateway auth.
// Must match the Hyperspeed provisioning gateway canonical signing payload (private edge implementation).
func CanonicalSignPayload(timestampUnix int64, method, path string, body []byte) string {
	h := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(h[:])
	return fmt.Sprintf("%d\n%s\n%s\n%s", timestampUnix, strings.ToUpper(strings.TrimSpace(method)), path, bodyHash)
}

// SignGatewayRequest returns hex-encoded HMAC-SHA256 of CanonicalSignPayload.
func SignGatewayRequest(installSecret string, timestampUnix int64, method, path string, body []byte) string {
	msg := CanonicalSignPayload(timestampUnix, method, path, body)
	mac := hmac.New(sha256.New, []byte(installSecret))
	_, _ = mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}
