package provisioning

import "testing"

func TestGiftedSubdomainFromIntendedURL(t *testing.T) {
	slug, err := GiftedSubdomainFromIntendedURL("https://www.acme.hyperspeedapp.com")
	if err != nil || slug != "acme" {
		t.Fatalf("got %q %v want acme", slug, err)
	}
	slug, err = GiftedSubdomainFromIntendedURL("https://acme.hyperspeedapp.com")
	if err != nil || slug != "acme" {
		t.Fatalf("legacy: got %q %v want acme", slug, err)
	}
	if _, err := GiftedSubdomainFromIntendedURL("http://www.acme.hyperspeedapp.com"); err == nil {
		t.Fatal("expected error for http")
	}
	if _, err := GiftedSubdomainFromIntendedURL("https://evil.com"); err == nil {
		t.Fatal("expected error for wrong host")
	}
}
