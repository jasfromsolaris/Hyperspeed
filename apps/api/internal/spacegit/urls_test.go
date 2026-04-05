package spacegit

import "testing"

func TestAuthedHTTPSURL(t *testing.T) {
	u, err := AuthedHTTPSURL("https://github.com/acme/demo.git", "tok1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if u == "" {
		t.Fatal("empty url")
	}
	if _, err := AuthedHTTPSURL("http://github.com/acme/demo.git", "x"); err == nil {
		t.Fatal("expected error for non-https")
	}
}
