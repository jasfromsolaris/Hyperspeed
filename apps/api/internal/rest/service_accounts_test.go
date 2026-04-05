package rest

import (
	"testing"

	"hyperspeed/api/internal/store"
)

func TestValidateServiceAccountProviderFields(t *testing.T) {
	m := "nvidia/nemotron-3-super-120b-a12b:free"
	if err := validateServiceAccountProviderFields(store.ProviderOpenRouter, &m, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateServiceAccountProviderFields(store.ProviderOpenRouter, nil, nil); err == nil {
		t.Fatal("expected error without model")
	}
	repo := "https://github.com/a/b"
	if err := validateServiceAccountProviderFields(store.ProviderCursor, nil, &repo); err != nil {
		t.Fatal(err)
	}
	if err := validateServiceAccountProviderFields(store.ProviderCursor, nil, nil); err != nil {
		t.Fatalf("cursor without repo URL is allowed (resolved from space IDE Git at launch): %v", err)
	}
	if err := validateServiceAccountProviderFields("bogus", &m, nil); err == nil {
		t.Fatal("expected error for bad provider")
	}
}
