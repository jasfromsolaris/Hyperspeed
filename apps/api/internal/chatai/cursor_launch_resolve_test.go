package chatai

import (
	"strings"
	"testing"

	"hyperspeed/api/internal/store"
)

func strPtr(s string) *string { return &s }

func TestResolveCursorRepoInputs(t *testing.T) {
	link := &store.SpaceGitLink{
		RemoteURL: "https://github.com/o/r.git",
		Branch:    "develop",
	}
	t.Run("sa_repo_wins_over_space", func(t *testing.T) {
		repo, ref, err := resolveCursorRepoInputs(store.ServiceAccount{
			CursorDefaultRepoURL: strPtr("https://github.com/a/b.git"),
			CursorDefaultRef:     strPtr("release"),
		}, link)
		if err != nil {
			t.Fatal(err)
		}
		if repo != "https://github.com/a/b.git" || ref != "release" {
			t.Fatalf("got %q %q", repo, ref)
		}
	})
	t.Run("space_repo_when_sa_empty", func(t *testing.T) {
		repo, ref, err := resolveCursorRepoInputs(store.ServiceAccount{}, link)
		if err != nil {
			t.Fatal(err)
		}
		if repo != "https://github.com/o/r.git" || ref != "develop" {
			t.Fatalf("got %q %q", repo, ref)
		}
	})
	t.Run("sa_repo_space_ref_when_sa_ref_empty", func(t *testing.T) {
		repo, ref, err := resolveCursorRepoInputs(store.ServiceAccount{
			CursorDefaultRepoURL: strPtr("https://github.com/a/b.git"),
		}, link)
		if err != nil {
			t.Fatal(err)
		}
		if repo != "https://github.com/a/b.git" || ref != "develop" {
			t.Fatalf("got %q %q", repo, ref)
		}
	})
	t.Run("main_when_no_branch", func(t *testing.T) {
		plain := &store.SpaceGitLink{RemoteURL: "https://x/y", Branch: ""}
		_, ref, err := resolveCursorRepoInputs(store.ServiceAccount{}, plain)
		if err != nil {
			t.Fatal(err)
		}
		if ref != "main" {
			t.Fatalf("ref %q", ref)
		}
	})
	t.Run("err_when_no_repo_anywhere", func(t *testing.T) {
		_, _, err := resolveCursorRepoInputs(store.ServiceAccount{}, nil)
		if err != ErrNoCursorRepoForLaunch {
			t.Fatalf("err %v", err)
		}
		_, _, err = resolveCursorRepoInputs(store.ServiceAccount{}, &store.SpaceGitLink{RemoteURL: "  "})
		if err != ErrNoCursorRepoForLaunch {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("trim_spaces", func(t *testing.T) {
		repo, ref, err := resolveCursorRepoInputs(store.ServiceAccount{
			CursorDefaultRepoURL: strPtr("  https://a/b  "),
			CursorDefaultRef:     strPtr("  topic  "),
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if repo != "https://a/b" || ref != "topic" {
			t.Fatalf("got %q %q", repo, ref)
		}
	})
}

func TestResolveCursorRepoInputs_branchWhitespace(t *testing.T) {
	link := &store.SpaceGitLink{RemoteURL: "https://x/y", Branch: "  main  "}
	_, ref, err := resolveCursorRepoInputs(store.ServiceAccount{}, link)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(ref) != ref {
		t.Fatalf("expected trimmed ref, got %q", ref)
	}
}
