package provisioning

import (
	"testing"
)

// Test vectors must stay aligned with the Hyperspeed provisioning gateway (private edge repo).
func TestCanonicalSignPayload_Vector(t *testing.T) {
	const (
		ts       int64 = 1700000000
		method         = "POST"
		path           = "/v1/claims"
	)
	body := []byte(`{"slug":"acme","ipv4":"203.0.113.1"}`)
	want := "1700000000\nPOST\n/v1/claims\n349a1e120607a9074611fefdb92705eb3ab72cb158f304aa6035747e67d0be28"
	got := CanonicalSignPayload(ts, method, path, body)
	if got != want {
		t.Fatalf("canonical mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestSignGatewayRequest_Vector(t *testing.T) {
	const (
		ts       int64 = 1700000000
		method         = "POST"
		path           = "/v1/claims"
		secret         = "test-secret-key-for-vectors"
		wantSig        = "07714ec0d38ecb83efd662d8fc08e105000861beba6ef3fad043de968587d188"
	)
	body := []byte(`{"slug":"acme","ipv4":"203.0.113.1"}`)
	got := SignGatewayRequest(secret, ts, method, path, body)
	if got != wantSig {
		t.Fatalf("signature mismatch\n got: %q\nwant: %q", got, wantSig)
	}
}
