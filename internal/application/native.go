package application

import (
	"context"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

type WriteRequest struct {
	Host              domain.Host   `json:"host" enum:"codex,deepseek,claude,zcode,cli" description:"Actual owning Host: cli, codex, deepseek, claude or zcode. This field does not authenticate user authorization."`
	RepositoryPath    string        `json:"repository_path" description:"Absolute worktree root used for claim discovery and native path preflight."`
	ToolName          string        `json:"tool_name" description:"Actual registered structured-write tool name."`
	Paths             []string      `json:"paths" description:"All actual absolute target paths parsed from the structured operation."`
	IntentDigest      domain.Digest `json:"intent_digest" description:"Host-computed SHA-256 of the original structured tool intent, not a permission token."`
	PathParseComplete bool          `json:"path_parse_complete" description:"False when any target could not be parsed; an active Task then refuses the write."`
}
type WriteDecision struct {
	Decision       string `json:"decision" enum:"allow,deny" description:"Path preflight outcome only; original Host permissions remain authoritative."`
	Coverage       string `json:"coverage" enum:"no_active_task,structured_paths" description:"no_active_task does not claim Task constraint coverage."`
	TaskID         string `json:"task_id,omitempty" description:"Claimed Core Task, when present."`
	TaskRevision   uint64 `json:"task_revision,omitempty" description:"Saved Task revision used by this preflight."`
	Reason         string `json:"reason,omitempty" description:"Concrete reason for refusal; correct the actual request rather than switching tools to bypass it."`
	SemanticReview string `json:"semantic_review,omitempty" description:"Content semantics still require checkpoint review."`
}
type WorkspaceSelection struct {
	Key            domain.RepositoryKey `json:"key" description:"Stable lowercase repository key."`
	RepositoryPath string               `json:"repository_path" description:"Existing absolute worktree root; Core canonicalizes it."`
}
type WorkspaceRequest struct {
	RepositoryPath string               `json:"repository_path,omitempty" description:"One absolute root; use this or repositories, never both."`
	Repositories   []WorkspaceSelection `json:"repositories,omitempty" description:"One to eight selected roots with unique keys; every result is returned in this order."`
}
type WorkspaceClaim struct {
	Key            domain.RepositoryKey `json:"key" description:"Input repository key, primary for a singular root."`
	RepositoryPath string               `json:"repository_path" description:"Observed canonical root."`
	ActiveTaskID   *string              `json:"active_task_id" description:"Existing active Task; null means no claim, not proof that an earlier open never occurred."`
}
type WorkspaceAvailability struct {
	Available    bool             `json:"available" description:"True only when every selected root has no active claim; not a write authorization."`
	Repositories []WorkspaceClaim `json:"repositories" description:"Per-root claims, including all conflicts rather than only the first."`
}

/**
 * WorkspaceAvailable discovers Core ownership for a meaningful continuation,
 * without creating another cursor or deciding whether the user's goal changed.
 */
func (s *Service) WorkspaceAvailable(ctx context.Context, q WorkspaceRequest) (WorkspaceAvailability, error) {
	roots := q.Repositories
	if q.RepositoryPath != "" {
		if len(roots) > 0 {
			return WorkspaceAvailability{}, domain.Invalid("repositories", "use repository_path or repositories, not both")
		}
		roots = []WorkspaceSelection{{Key: "primary", RepositoryPath: q.RepositoryPath}}
	}
	if len(roots) < 1 || len(roots) > 8 {
		return WorkspaceAvailability{}, domain.Invalid("repositories", "requires one to eight selected roots")
	}
	result := WorkspaceAvailability{Available: true, Repositories: []WorkspaceClaim{}}
	keys, paths := map[domain.RepositoryKey]bool{}, map[string]bool{}
	for _, root := range roots {
		if !root.Key.IsValid() || keys[root.Key] {
			return WorkspaceAvailability{}, domain.Invalid("repositories.key", "requires unique valid repository keys")
		}
		keys[root.Key] = true
		canonical, _, err := s.Observer.IdentifyWorkspace(ctx, root.RepositoryPath)
		if err != nil {
			return WorkspaceAvailability{}, err
		}
		if paths[canonical] {
			return WorkspaceAvailability{}, domain.Invalid("repositories", "duplicate canonical workspace")
		}
		paths[canonical] = true
		id, err := s.Store.Claim(ctx, canonical)
		if err != nil {
			return WorkspaceAvailability{}, err
		}
		entry := WorkspaceClaim{Key: root.Key, RepositoryPath: canonical}
		if id != "" {
			entry.ActiveTaskID = &id
			result.Available = false
		}
		result.Repositories = append(result.Repositories, entry)
	}
	return result, nil
}
