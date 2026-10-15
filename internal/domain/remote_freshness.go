package domain

import (
	"slices"
	"strings"
	"time"
)

// Fetched is Host testimony; Core verifies retained local facts, not remote server freshness.
type RemoteFreshness struct {
	Head       string     `json:"head" description:"实际观察到的HEAD提交。"`
	Upstream   string     `json:"upstream" description:"该处明确指代的来源或任务分支上游引用；null表示没有，不能推定IDE目标。"`
	RemoteHead string     `json:"remote_head" description:"实际获取的追踪提交，未获取时为空，不能伪称最新。"`
	State      string     `json:"state" description:"远端观察为current/ahead/behind/diverged/not_checked/not_fetched/no_upstream/fetch_failed等；安装target为安装状态。"`
	Fetched    bool       `json:"fetched" description:"本次观察是否实际fetch成功。"`
	RemoteName string     `json:"remote_name" description:"remote来源的真实远端名；local来源为空。"`
	RemoteRef  string     `json:"remote_ref" description:"选定远端上的完整ref，例如refs/heads/main。"`
	FetchedAt  *time.Time `json:"fetched_at,omitempty" description:"UTC time of actual successful fetch, when performed."`
	CheckedAt  time.Time  `json:"checked_at" description:"此次观察时间，UTC。"`
	Basis      string     `json:"basis" description:"与实际观察、当前目标或检查关联的具体判断理由。"`
}
type RepositoryFreshness struct {
	Key         RepositoryKey   `json:"key" description:"仓库稳定key，1..128个小写ASCII字母数字及._-，以字母数字开头。"`
	Observation RemoteFreshness `json:"observation" description:"Host remote observation verified against actual Git state."`
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
