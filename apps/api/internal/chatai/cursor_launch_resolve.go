package chatai

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/store"
)

// ErrNoCursorRepoForLaunch means neither the service account nor the space git link provides a remote URL.
var ErrNoCursorRepoForLaunch = errors.New("no cursor repository URL from service account or space git link")

// resolveCursorRepoInputs applies precedence for RepositoryURL and Ref. link may be nil when the space has no row.
func resolveCursorRepoInputs(sa store.ServiceAccount, link *store.SpaceGitLink) (repoURL string, ref string, err error) {
	if sa.CursorDefaultRepoURL != nil && strings.TrimSpace(*sa.CursorDefaultRepoURL) != "" {
		repoURL = strings.TrimSpace(*sa.CursorDefaultRepoURL)
	} else if link != nil && strings.TrimSpace(link.RemoteURL) != "" {
		repoURL = strings.TrimSpace(link.RemoteURL)
	}
	if repoURL == "" {
		return "", "", ErrNoCursorRepoForLaunch
	}

	if sa.CursorDefaultRef != nil && strings.TrimSpace(*sa.CursorDefaultRef) != "" {
		ref = strings.TrimSpace(*sa.CursorDefaultRef)
	} else if link != nil && strings.TrimSpace(link.Branch) != "" {
		ref = strings.TrimSpace(link.Branch)
	} else {
		ref = "main"
	}
	return repoURL, ref, nil
}

// ResolveCursorRepoForLaunch picks RepositoryURL and Ref for Cursor Cloud Agents.
// Precedence: explicit service account URL over space_git_links.remote_url; explicit ref over space branch, else main.
func ResolveCursorRepoForLaunch(ctx context.Context, st *store.Store, spaceID uuid.UUID, sa store.ServiceAccount) (repoURL string, ref string, err error) {
	l, err := st.GetSpaceGitLink(ctx, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return resolveCursorRepoInputs(sa, nil)
		}
		return "", "", err
	}
	return resolveCursorRepoInputs(sa, &l)
}
