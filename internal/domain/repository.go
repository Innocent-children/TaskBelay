package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"
	"strings"
	"time"
)

type RepositoryKey string

const (
	DefaultPrimaryRepositoryKey RepositoryKey = "primary"
	repositoryScopeDigestDomain               = "taskbelay/repository-scope-binding"
)

func (k RepositoryKey) IsValid() bool {
	if len(k) == 0 || len(k) > MaxRepositoryKeyBytes {
		return false
	}
	for i, character := range []byte(k) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || i > 0 && (character == '.' || character == '_' || character == '-') {
			continue
		}
		return false
	}
	return true
}

type WorkspaceMode string

const (
	WorkspaceModeNewBranch         WorkspaceMode = "new_branch"
	WorkspaceModeCurrentBranch     WorkspaceMode = "current_branch"
	WorkspaceModeDedicatedWorktree WorkspaceMode = "dedicated_worktree"
)

func (m WorkspaceMode) IsValid() bool {
	return m == WorkspaceModeNewBranch || m == WorkspaceModeCurrentBranch || m == WorkspaceModeDedicatedWorktree
}

// WorkspaceOrigin is the immutable origin of one Task worktree. The Host
// supplies the selection fields and Core fills the three observed location
// fields before the Task is created.
type WorkspaceOrigin struct {
	Mode                        WorkspaceMode `json:"mode"`
	SourceType                  string        `json:"source_type"`
	CarryChanges                bool          `json:"carry_changes"`
	RemoteName                  string        `json:"remote_name"`
	BaseBranch                  string        `json:"base_branch"`
	BaseCommit                  string        `json:"base_commit"`
	TaskBranch                  string        `json:"task_branch"`
	SourceRepositoryGroupDigest Digest        `json:"source_repository_group_digest"`
	CanonicalWorktreeRoot       string        `json:"canonical_worktree_root"`
	WorktreeGitDirDigest        Digest        `json:"worktree_git_dir_digest"`
	ProvisioningReceiptID       ID            `json:"provisioning_receipt_id"`
}

func (o WorkspaceOrigin) Validate() error {
	if !o.Mode.IsValid() || !ValidWorkspaceSource(o.SourceType, o.RemoteName, o.CarryChanges) || !validBranchName(o.BaseBranch) || !validBranchName(o.TaskBranch) ||
		validateObjectID(o.BaseCommit) != nil || validateDigest(o.SourceRepositoryGroupDigest) != nil ||
		validateCanonicalPath(o.CanonicalWorktreeRoot) != nil || validateDigest(o.WorktreeGitDirDigest) != nil ||
		validateID(o.ProvisioningReceiptID) != nil {
		return ErrInvalidArgument
	}
	if o.Mode != WorkspaceModeDedicatedWorktree && (o.SourceType != "local" ||
		o.Mode == WorkspaceModeCurrentBranch && o.BaseBranch != o.TaskBranch ||
		o.Mode == WorkspaceModeNewBranch && o.BaseBranch == o.TaskBranch) {
		return ErrInvalidArgument
	}
	return nil
}

// ValidWorkspaceSource validates the confirmed source and content selection.
func ValidWorkspaceSource(source, remote string, carry bool) bool {
	return source == "local" && remote == "" || source == "remote" && validRemoteName(remote) && !carry
}

func validRemoteName(value string) bool {
	if value == "" || len(value) > MaxIdentifierBytes || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validBranchName(value string) bool {
	if value == "" || len(value) > MaxRepositoryPathBytes || strings.TrimSpace(value) != value || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.ContainsAny(value, " ~^:?*[\\") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return value != "@"
}

type RepositoryHistoryRelation string

const (
	RepositoryHistoryExact         RepositoryHistoryRelation = "exact"
	RepositoryHistoryLinearAdvance RepositoryHistoryRelation = "linear_advance"
	RepositoryHistoryRewind        RepositoryHistoryRelation = "rewind"
	RepositoryHistoryRewrite       RepositoryHistoryRelation = "rewrite"
	RepositoryHistoryBranchChanged RepositoryHistoryRelation = "branch_changed"
	RepositoryHistoryDetached      RepositoryHistoryRelation = "detached"
)

func (r RepositoryHistoryRelation) IsValid() bool {
	switch r {
	case RepositoryHistoryExact, RepositoryHistoryLinearAdvance, RepositoryHistoryRewind,
		RepositoryHistoryRewrite, RepositoryHistoryBranchChanged, RepositoryHistoryDetached:
		return true
	default:
		return false
	}
}

type RepositoryChangeType string

const (
	RepositoryChangeAdded    RepositoryChangeType = "added"
	RepositoryChangeModified RepositoryChangeType = "modified"
	RepositoryChangeDeleted  RepositoryChangeType = "deleted"
	RepositoryChangeTypeMode RepositoryChangeType = "type_changed"
)

func (t RepositoryChangeType) IsValid() bool {
	return t == RepositoryChangeAdded || t == RepositoryChangeModified || t == RepositoryChangeDeleted || t == RepositoryChangeTypeMode
}

// RepositoryChangedEntry is a bounded path observation. ContentDigest
// identifies the current path state, including deletion, without file bytes.
type RepositoryChangedEntry struct {
	Path                  string               `json:"path"`
	ChangeType            RepositoryChangeType `json:"change_type"`
	FileMode              string               `json:"file_mode"`
	Gitlink               bool                 `json:"gitlink"`
	BaseMode              string               `json:"base_mode"`
	BaseContentDigest     Digest               `json:"base_content_digest"`
	IndexMode             string               `json:"index_mode"`
	IndexContentDigest    Digest               `json:"index_content_digest"`
	WorktreeMode          string               `json:"worktree_mode"`
	WorktreeContentDigest Digest               `json:"worktree_content_digest"`
	ContentDigest         Digest               `json:"content_digest"`
}

func (e RepositoryChangedEntry) Validate() error {
	if validateRepositoryRelativePath(e.Path) != nil || !e.ChangeType.IsValid() ||
		requireNormalizedText(e.FileMode, MaxIdentifierBytes, true) != nil || requireNormalizedText(e.BaseMode, MaxIdentifierBytes, true) != nil || requireNormalizedText(e.IndexMode, MaxIdentifierBytes, true) != nil || requireNormalizedText(e.WorktreeMode, MaxIdentifierBytes, true) != nil || !e.BaseContentDigest.IsValid() || !e.IndexContentDigest.IsValid() || !e.WorktreeContentDigest.IsValid() || !e.ContentDigest.IsValid() ||
		e.Gitlink != (e.FileMode == "160000") {
		return ErrInvalidArgument
	}
	return nil
}

// RepositoryBinding is one immutable observation of a provisioned Task worktree.
type RepositoryBinding struct {
	WorktreeInstanceDigest Digest                    `json:"worktree_instance_digest"`
	IdentityDigest         Digest                    `json:"identity_digest"`
	HistoryDigest          Digest                    `json:"history_digest"`
	ContentDigest          Digest                    `json:"content_digest"`
	CurrentBranch          *string                   `json:"current_branch"`
	Detached               bool                      `json:"detached"`
	CurrentHead            string                    `json:"current_head"`
	HeadTree               string                    `json:"head_tree"`
	HistoryRelation        RepositoryHistoryRelation `json:"history_relation"`
	BaseCommitAncestor     bool                      `json:"base_commit_ancestor"`
	ChangedEntries         []RepositoryChangedEntry  `json:"changed_entries"`
	TaskSurface            []RepositoryChangedEntry  `json:"task_surface"`
	ObservedAt             time.Time                 `json:"observed_at"`
	BindingDigest          Digest                    `json:"binding_digest"`
}

type RepositoryScopeEntry struct {
	Key            RepositoryKey     `json:"key"`
	Origin         WorkspaceOrigin   `json:"workspace_origin"`
	Binding        RepositoryBinding `json:"binding"`
	RemoteBaseline *RemoteFreshness  `json:"remote_baseline,omitempty"`
	RemoteLatest   *RemoteFreshness  `json:"remote_latest,omitempty"`
}

func (e RepositoryScopeEntry) Validate() error {
	for _, f := range []*RemoteFreshness{e.RemoteBaseline, e.RemoteLatest} {
		if f != nil {
			if err := f.Validate(); err != nil {
				return err
			}
		}
	}
	if !e.Key.IsValid() || e.Origin.Validate() != nil || e.Binding.Validate() != nil {
		return ErrInvalidArgument
	}
	return nil
}

func (e RepositoryScopeEntry) Clone() RepositoryScopeEntry {
	e.Binding = e.Binding.Clone()
	return e
}

func NormalizeRepositoryScope(primaryKey RepositoryKey, primaryOrigin WorkspaceOrigin, primary RepositoryBinding, additional []RepositoryScopeEntry) (RepositoryKey, []RepositoryScopeEntry, error) {
	if primaryKey == "" {
		primaryKey = DefaultPrimaryRepositoryKey
	}
	entries := make([]RepositoryScopeEntry, len(additional))
	for i := range additional {
		entries[i] = additional[i].Clone()
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	if err := validateRepositoryScope(primaryKey, primaryOrigin, primary, entries); err != nil {
		return "", nil, err
	}
	return primaryKey, entries, nil
}

func validateRepositoryScope(primaryKey RepositoryKey, primaryOrigin WorkspaceOrigin, primary RepositoryBinding, additional []RepositoryScopeEntry) error {
	if !primaryKey.IsValid() || primaryOrigin.Validate() != nil || primary.Validate() != nil || len(additional) > MaxAdditionalRepositories {
		return ErrInvalidArgument
	}
	keys := map[RepositoryKey]bool{primaryKey: true}
	instances := map[Digest]bool{primary.WorktreeInstanceDigest: true}
	previous := RepositoryKey("")
	for _, entry := range additional {
		if entry.Validate() != nil || keys[entry.Key] || instances[entry.Binding.WorktreeInstanceDigest] || previous != "" && entry.Key <= previous {
			return ErrInvalidArgument
		}
		keys[entry.Key] = true
		instances[entry.Binding.WorktreeInstanceDigest] = true
		previous = entry.Key
	}
	return nil
}

type WorkspaceDigests struct {
	Binding  Digest
	Identity Digest
	History  Digest
	Content  Digest
}

func effectiveRepositoryDigests(primaryKey RepositoryKey, primaryOrigin WorkspaceOrigin, primary RepositoryBinding, additional []RepositoryScopeEntry) (WorkspaceDigests, error) {
	if primaryKey == "" {
		primaryKey = DefaultPrimaryRepositoryKey
	}
	if err := validateRepositoryScope(primaryKey, primaryOrigin, primary, additional); err != nil {
		return WorkspaceDigests{}, err
	}
	return WorkspaceDigests{
		Binding:  digestRepositoryScope(primaryKey, primary.BindingDigest, additional, func(e RepositoryScopeEntry) Digest { return e.Binding.BindingDigest }, "binding"),
		Identity: digestRepositoryScope(primaryKey, primary.IdentityDigest, additional, func(e RepositoryScopeEntry) Digest { return e.Binding.IdentityDigest }, "identity"),
		History:  digestRepositoryScope(primaryKey, primary.HistoryDigest, additional, func(e RepositoryScopeEntry) Digest { return e.Binding.HistoryDigest }, "history"),
		Content:  digestRepositoryScope(primaryKey, primary.ContentDigest, additional, func(e RepositoryScopeEntry) Digest { return e.Binding.ContentDigest }, "content"),
	}, nil
}

func digestRepositoryScope(primaryKey RepositoryKey, primary Digest, additional []RepositoryScopeEntry, selectDigest func(RepositoryScopeEntry) Digest, kind string) Digest {
	if len(additional) == 0 {
		return primary
	}
	digest := sha256.New()
	writeRepositoryScopeDigestField(digest, []byte(repositoryScopeDigestDomain+"/"+kind))
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(additional)+1))
	writeRepositoryScopeDigestField(digest, count[:])
	writeRepositoryScopeDigestEntry(digest, "primary", primaryKey, primary)
	for _, entry := range additional {
		writeRepositoryScopeDigestEntry(digest, "additional", entry.Key, selectDigest(entry))
	}
	return Digest(hex.EncodeToString(digest.Sum(nil)))
}

func writeRepositoryScopeDigestEntry(destination hash.Hash, role string, key RepositoryKey, digest Digest) {
	writeRepositoryScopeDigestField(destination, []byte(role))
	writeRepositoryScopeDigestField(destination, []byte(key))
	writeRepositoryScopeDigestField(destination, []byte(digest))
}

func writeRepositoryScopeDigestField(destination hash.Hash, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = destination.Write(length[:])
	_, _ = destination.Write(value)
}

func (b RepositoryBinding) Validate() error {
	if !b.WorktreeInstanceDigest.IsValid() || !b.IdentityDigest.IsValid() || !b.HistoryDigest.IsValid() ||
		!b.ContentDigest.IsValid() || !b.BindingDigest.IsValid() || validateUTC(b.ObservedAt) != nil ||
		validateObjectID(b.CurrentHead) != nil || validateObjectID(b.HeadTree) != nil || !b.HistoryRelation.IsValid() {
		return ErrInvalidArgument
	}
	if b.Detached != (b.CurrentBranch == nil) || b.CurrentBranch != nil && requireNormalizedText(*b.CurrentBranch, MaxRepositoryPathBytes, true) != nil {
		return ErrInvalidArgument
	}
	for _, entries := range [][]RepositoryChangedEntry{b.ChangedEntries, b.TaskSurface} {
		if len(entries) > MaxFingerprintPaths {
			return ErrInvalidArgument
		}
		for i, entry := range entries {
			if entry.Validate() != nil || i > 0 && entries[i-1].Path >= entry.Path {
				return ErrInvalidArgument
			}
		}
	}
	return nil
}

func (b RepositoryBinding) Clone() RepositoryBinding {
	b.CurrentBranch = cloneStringPointer(b.CurrentBranch)
	b.ChangedEntries = append([]RepositoryChangedEntry(nil), b.ChangedEntries...)
	b.TaskSurface = append([]RepositoryChangedEntry(nil), b.TaskSurface...)
	return b
}

func RepositoryChangedPaths(entries []RepositoryChangedEntry) []string {
	paths := make([]string, len(entries))
	for i := range entries {
		paths[i] = entries[i].Path
	}
	return paths
}

// RepositoryScopeTaskSurfacePaths projects the exact current task surface into
// the repository-qualified path format retained by ProcessTask.
func RepositoryScopeTaskSurfacePaths(primaryKey RepositoryKey, primary RepositoryBinding, additional []RepositoryScopeEntry) []string {
	if primaryKey == "" {
		primaryKey = DefaultPrimaryRepositoryKey
	}

	paths := make([]string, 0, len(primary.TaskSurface))
	primaryPrefix := ""
	if len(additional) != 0 {
		primaryPrefix = string(primaryKey) + repositoryPathSeparator
	}
	for _, entry := range primary.TaskSurface {
		paths = append(paths, primaryPrefix+entry.Path)
	}
	for _, repository := range additional {
		prefix := string(repository.Key) + repositoryPathSeparator
		for _, entry := range repository.Binding.TaskSurface {
			paths = append(paths, prefix+entry.Path)
		}
	}
	sort.Strings(paths)
	return paths
}
