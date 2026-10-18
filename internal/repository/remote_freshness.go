package repository

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"strconv"
	"strings"
	"time"
)

func (o *GitObserver) VerifyRemoteFreshness(ctx context.Context, root string, b domain.RepositoryBinding, origin domain.WorkspaceOrigin, reported *domain.RemoteFreshness) (domain.RemoteFreshness, error) {
	if reported == nil {
		return domain.RemoteFreshness{Head: b.CurrentHead, State: "not_checked", CheckedAt: time.Now().UTC(), Basis: "Host did not supply an upstream fetch observation; remote freshness is unknown."}, nil
	}
	f := *reported
	if e := f.Validate(); e != nil {
		return f, e
	}
	if f.Head != b.CurrentHead || f.CheckedAt.After(time.Now().UTC().Add(time.Minute)) {
		return f, domain.Fail("REMOTE_OBSERVATION_STALE", "remote_freshness", "reported local head/time does not match the bound observation")
	}
	if !f.Fetched {
		return f, nil
	}
	// The source is immutable Task authority. A task branch's own upstream is irrelevant.
	config := []string{"refs/remotes/" + origin.RemoteName + "/" + origin.BaseBranch, origin.RemoteName, "refs/heads/" + origin.BaseBranch}
	if origin.SourceType == "local" {
		up, err := o.required(ctx, gitShowUpstream, root, origin.BaseBranch)
		if err != nil {
			return f, err
		}
		config = nil
		for _, line := range strings.Split(strings.TrimSpace(string(up.stdout)), "\n") {
			fields := strings.Split(line, "\x00")
			if len(fields) == 4 && fields[0] == "refs/heads/"+origin.BaseBranch {
				config = fields[1:]
				break
			}
		}
	} else {
		remotes, err := o.required(ctx, gitShowRemotes, root, "")
		if err != nil {
			return f, err
		}
		found := false
		for _, name := range strings.Fields(string(remotes.stdout)) {
			if name == origin.RemoteName {
				found = true
			}
		}
		if !found {
			return f, domain.Fail("REMOTE_OBSERVATION_STALE", "remote_freshness.remote_name", "bound source remote no longer exists")
		}
	}
	if len(config) != 3 || config[0] != f.Upstream || config[1] != f.RemoteName || config[2] != f.RemoteRef {
		return f, domain.Fail("REMOTE_OBSERVATION_STALE", "remote_freshness.upstream", "reported upstream is not the bound source upstream")
	}
	commit, e := ReadGit(ctx, root, "show", "-s", "--format=%H", f.Upstream)
	if e != nil || strings.TrimSpace(string(commit)) != f.RemoteHead {
		return f, domain.Fail("REMOTE_OBSERVATION_STALE", "remote_freshness.remote_head", "local remote-tracking ref does not match the reported fetched commit")
	}
	counts, e := o.required(ctx, gitCompareUpstream, root, f.Upstream)
	if e != nil {
		return f, e
	}
	values := strings.Fields(string(counts.stdout))
	if len(values) != 2 {
		return f, ErrGitObservation
	}
	a, ae := strconv.ParseInt(values[0], 10, 64)
	z, ze := strconv.ParseInt(values[1], 10, 64)
	if ae != nil || ze != nil || a < 0 || z < 0 {
		return f, ErrGitObservation
	}
	state := "current"
	if a > 0 && z > 0 {
		state = "diverged"
	} else if a > 0 {
		state = "ahead"
	} else if z > 0 {
		state = "behind"
	}
	if state != f.State {
		return f, domain.Fail("REMOTE_OBSERVATION_STALE", "remote_freshness.state", "upstream comparison changed after Host assessment")
	}
	return f, nil
}
