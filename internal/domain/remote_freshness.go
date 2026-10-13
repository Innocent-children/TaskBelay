package domain

import (
	"slices"
	"strings"
	"time"
)

// Fetched is Host testimony; Core verifies retained local facts, not remote server freshness.
type RemoteFreshness struct {
	Head       string     `json:"head"`
	Upstream   string     `json:"upstream"`
	RemoteHead string     `json:"remote_head"`
	State      string     `json:"state"`
	Fetched    bool       `json:"fetched"`
	RemoteName string     `json:"remote_name"`
	RemoteRef  string     `json:"remote_ref"`
	FetchedAt  *time.Time `json:"fetched_at,omitempty"`
	CheckedAt  time.Time  `json:"checked_at"`
	Basis      string     `json:"basis"`
}
type RepositoryFreshness struct {
	Key         RepositoryKey   `json:"key"`
	Observation RemoteFreshness `json:"observation"`
}

func (f RemoteFreshness) Validate() error {
	if validateObjectID(f.Head) != nil || validateUTC(f.CheckedAt) != nil || Text("remote_freshness.basis", f.Basis) != nil || !slices.Contains([]string{"not_checked", "not_fetched", "fetch_failed", "no_upstream", "detached", "current", "ahead", "behind", "diverged"}, f.State) {
		return Invalid("remote_freshness", "requires explicit fetch state, basis, local head and UTC time")
	}
	if f.Upstream != "" && (!strings.HasPrefix(f.Upstream, "refs/remotes/") || ScopedPath(f.Upstream) != nil) {
		return Invalid("remote_freshness.upstream", "requires a canonical remote-tracking ref")
	}
	if f.Fetched {
		if f.Upstream == "" || !ValidWorkspaceSource("remote", f.RemoteName, false) || !strings.HasPrefix(f.RemoteRef, "refs/heads/") || ScopedPath(f.RemoteRef) != nil || f.FetchedAt == nil || validateUTC(*f.FetchedAt) != nil || f.CheckedAt.Before(*f.FetchedAt) || validateObjectID(f.RemoteHead) != nil || !slices.Contains([]string{"current", "ahead", "behind", "diverged"}, f.State) {
			return Invalid("remote_freshness", "fetched observation requires an upstream, commit and comparison")
		}
	} else if f.FetchedAt != nil || slices.Contains([]string{"current", "ahead", "behind", "diverged"}, f.State) {
		return Invalid("remote_freshness", "cached refs cannot establish fetched state")
	}
	return nil
}
