package secrets

import (
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	plain := "cursor_api_key_test_value"
	enc, err := EncryptString(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptString(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("decrypt = %q want %q", got, plain)
	}
}
