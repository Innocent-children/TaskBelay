package workflow

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func DecodeBlockerResolutionPayload(raw []byte) (domain.BlockerResolutionPayload, json.RawMessage, error) {
	if len(raw) == 0 || !json.Valid(raw) || rejectDuplicateMembers(raw) != nil {
		return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "The blocker payload must be one valid JSON object without duplicate members.")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload domain.BlockerResolutionPayload
	if decoder.Decode(&payload) != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		!payload.BlockerID.IsValid() || payload.Condition.Validate() != nil || !payload.ObservedBindingDigest.IsValid() ||
		payload.FileScopeDecision != nil && payload.FileScopeDecision.Validate() != nil {
		return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "The blocker payload contains an unknown or mistyped member, invalid blocker identity, condition, binding digest or file-scope decision.")
	}
	if payload.Condition.Kind != domain.BlockerConditionResolveBranchRename && (payload.RenameID != "" || payload.RenameChoice != "") {
		return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "Rename inputs require a prepared branch-rename blocker.")
	}
	switch payload.Condition.Kind {
	case domain.BlockerConditionResolveBranchRename:
		if payload.RenameID != payload.Condition.RenameID || payload.RenameChoice != "complete" && payload.RenameChoice != "cancel" || payload.FileScopeDecision != nil || payload.HistoryResolution != nil || payload.RelocationID != "" || len(payload.RelocationDestinations) != 0 {
			return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "A branch-rename blocker requires its matching rename_id and complete or cancel, with no other decision types.")
		}
	case domain.BlockerConditionResolveFileScope:
		if payload.FileScopeDecision == nil || payload.HistoryResolution != nil || payload.RelocationID != "" || len(payload.RelocationDestinations) != 0 {
			return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "A file-scope blocker requires file_scope_decision and forbids history and relocation inputs.")
		}
	case domain.BlockerConditionResolveHistory:
		if payload.HistoryResolution == nil || payload.HistoryResolution.Validate() != nil || payload.FileScopeDecision != nil || payload.RelocationID != "" || len(payload.RelocationDestinations) != 0 {
			return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "A history blocker requires a valid history_resolution and forbids file-scope and relocation inputs.")
		}
	case domain.BlockerConditionResolveRelocation:
		if payload.RelocationID != payload.Condition.RelocationID || len(payload.RelocationDestinations) == 0 || len(payload.RelocationDestinations) > domain.MaxRepositoryScopeEntries || payload.FileScopeDecision != nil || payload.HistoryResolution != nil {
			return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "A relocation blocker requires its matching relocation_id, 1 to 8 destinations and no other decision types.")
		}
		seen := map[domain.RepositoryKey]bool{}
		for _, destination := range payload.RelocationDestinations {
			if !destination.Key.IsValid() || seen[destination.Key] || destination.RepositoryPath == "" || !filepath.IsAbs(destination.RepositoryPath) || filepath.Clean(destination.RepositoryPath) != destination.RepositoryPath {
				return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "Relocation destinations require unique valid repository keys and normalized absolute repository paths.")
			}
			seen[destination.Key] = true
		}
	default:
		if payload.FileScopeDecision != nil || payload.HistoryResolution != nil || payload.RelocationID != "" || len(payload.RelocationDestinations) != 0 {
			return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "This blocker condition does not accept file-scope, history or relocation decisions.")
		}
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return domain.BlockerResolutionPayload{}, nil, domain.WithExplanation(domain.ErrInvalidArgument, "The blocker-resolution payload could not be encoded as JSON.")
	}
	return payload, canonical, nil
}
