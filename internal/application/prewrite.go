package application

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
)

/**
 * PreWrite gates supported structured paths; it cannot inspect future semantics
 * or shell writes. The native Host permission still applies.
 */
func (s *Service) PreWrite(ctx context.Context, root string, paths []string, complete bool) (map[string]any, error) {
	canonical, instance, e := s.Observer.IdentifyWorkspace(ctx, root)
	if e != nil {
		if errors.Is(e, repository.ErrNotGitRepository) {
			return map[string]any{"decision": "allow", "coverage": "no_active_task"}, nil
		}
		return nil, e
	}
	id, e := s.Store.Claim(ctx, canonical)
	if e != nil {
		return nil, e
	}
	if id == "" {
		return map[string]any{"decision": "allow", "coverage": "no_active_task"}, nil
	}
	t, e := s.Store.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	deny := func(reason string) map[string]any {
		return map[string]any{"decision": "deny", "reason": reason, "task_id": id, "task_revision": t.Revision, "coverage": "structured_paths"}
	}
	for _, r := range t.Repositories {
		if r.Origin.CanonicalWorktreeRoot == canonical && r.Binding.WorktreeInstanceDigest != instance {
			return deny("Restore the original bound workspace instance before writing."), nil
		}
	}
	if e := requireExecutionAuthority(t); e != nil {
		return deny(e.Error()), nil
	}
	observed, e := s.observe(ctx, t)
	if e != nil {
		return deny("The bound workspace could not be observed before writing."), nil
	}
	for _, fact := range observed.Facts {
		if fact.Kind == "workspace_conflict" {
			return deny(fact.Detail), nil
		}
	}
	if !complete || len(paths) == 0 {
		return deny("Target paths were not fully parsed; inspect the tool operation before writing."), nil
	}
	for _, p := range paths {
		relative := ""
		for _, r := range t.Repositories {
			if r.Origin.CanonicalWorktreeRoot == canonical {
				relative = strings.TrimPrefix(p, canonical+string(os.PathSeparator))
				relative = strings.ReplaceAll(relative, "\\", "/")
				relative = t.QualifiedPath(r.Key, relative)
			}
		}
		if domain.ScopedPath(relative) != nil {
			return deny("Write target is outside the bound workspace."), nil
		}
		if !t.Authorization.AllowsPath(relative) {
			return deny("This path is outside the approved write scope."), nil
		}
		for _, pin := range t.Authorization.Pins {
			if pin.Path == relative {
				return deny("This content is pinned by the approved plan."), nil
			}
		}
	}
	return map[string]any{"decision": "allow", "task_id": id, "coverage": "structured_paths", "semantic_review": "required_at_checkpoint"}, nil
}
