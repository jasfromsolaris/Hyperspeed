package spacegit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const gitCmdTimeout = 5 * time.Minute

func runGit(ctx context.Context, dir string, args ...string) (string, string, error) {
	cctx, cancel := context.WithTimeout(ctx, gitCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// LsRemote checks that the remote is reachable and the branch exists (best-effort).
func LsRemote(ctx context.Context, authedURL, branch string) error {
	out, errOut, err := runGit(ctx, "", "ls-remote", "--heads", authedURL, "refs/heads/"+strings.TrimSpace(branch))
	if err != nil {
		if errOut != "" {
			return fmt.Errorf("%w: %s", err, errOut)
		}
		return err
	}
	if out == "" {
		return fmt.Errorf("branch %q not found on remote", branch)
	}
	return nil
}

// EnsureRepo clones into dir or fetch-resets if .git exists.
func EnsureRepo(ctx context.Context, authedURL, branch, dir string) error {
	gitDir := filepath.Join(dir, ".git")
	branch = strings.TrimSpace(branch)
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git binary not found in PATH")
	}
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		if _, _, err := runGit(ctx, dir, "fetch", "origin"); err != nil {
			return fmt.Errorf("git fetch: %w", err)
		}
		if _, _, err := runGit(ctx, dir, "checkout", "-B", branch, fmt.Sprintf("origin/%s", branch)); err != nil {
			return fmt.Errorf("git checkout: %w", err)
		}
		if _, errOut, err := runGit(ctx, dir, "reset", "--hard", fmt.Sprintf("origin/%s", branch)); err != nil {
			return fmt.Errorf("git reset: %v %s", err, errOut)
		}
		ConfigureLocalUser(ctx, dir)
		return nil
	}
	if _, errOut, err := runGit(ctx, "", "clone", "--depth", "50", "-b", branch, authedURL, dir); err != nil {
		return fmt.Errorf("git clone: %v %s", err, errOut)
	}
	ConfigureLocalUser(ctx, dir)
	return nil
}

// ConfigureLocalUser sets a neutral author for server-side commits.
func ConfigureLocalUser(ctx context.Context, dir string) {
	_, _, _ = runGit(ctx, dir, "config", "user.email", "hyperspeed@users.noreply.hyperspeed.local")
	_, _, _ = runGit(ctx, dir, "config", "user.name", "Hyperspeed")
}

// HeadSHA returns the current HEAD commit SHA.
func HeadSHA(ctx context.Context, dir string) (string, error) {
	out, errOut, err := runGit(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, errOut)
	}
	return out, nil
}

// WipeWorktree removes everything in dir except .git
func WipeWorktree(dir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// CommitAll commits all changes with the given message (fails if nothing to commit).
func CommitAll(ctx context.Context, dir, message string) error {
	_, _, _ = runGit(ctx, dir, "add", "-A")
	st, _, err := runGit(ctx, dir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(st) == "" {
		return fmt.Errorf("nothing to commit")
	}
	_, errOut, err := runGit(ctx, dir, "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("%v: %s", err, errOut)
	}
	return nil
}

// Push pushes to origin.
func Push(ctx context.Context, dir, branch, authedURL string) error {
	// Ensure remote URL carries credentials for this push (origin may be anonymous).
	if _, _, err := runGit(ctx, dir, "remote", "set-url", "origin", authedURL); err != nil {
		return fmt.Errorf("git remote set-url: %w", err)
	}
	_, errOut, err := runGit(ctx, dir, "push", "-u", "origin", branch)
	if err != nil {
		return fmt.Errorf("git push: %v %s", err, errOut)
	}
	return nil
}
