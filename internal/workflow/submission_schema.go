package workflow

import (
	"encoding/json"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

// ActionSubmissionSchema describes the semantic input shared by MCP and HTTP adapters.
func ActionSubmissionSchema(kind domain.ActionKind) (map[string]any, error) {
	node, err := NodeDefinitionForActionKind(StandardProcess(), kind)
	if err != nil || kind == domain.ActionResolveBlocker {
		return nil, domain.ErrInvalidArgument
	}
	result, err := SubmissionNodeResultSchema(kind)
	if err != nil {
		return nil, err
	}
	artifact := schemaObject([]string{"path", "digest", "summary"}, map[string]any{"path": schemaString(), "digest": schemaDigest(), "summary": schemaString()})
	artifacts := map[string]any{"other_process": map[string]any{"type": "array", "maxItems": domain.MaxArtifactReferencesPerAction, "items": artifact}}
	requiredArtifacts := []string{"other_process"}
	if _, ok := PrimaryArtifactRoleForNode(node.NodeID); ok {
		artifacts["current"] = map[string]any{"type": "array", "maxItems": domain.MaxArtifactReferencesPerAction, "items": artifact}
		requiredArtifacts = append([]string{"current"}, requiredArtifacts...)
	}
	methods := make(map[string]any, len(node.SemanticMethodSteps))
	requiredMethods := make([]string, len(node.SemanticMethodSteps))
	for index, step := range node.SemanticMethodSteps {
		requiredMethods[index] = string(step.StepID)
		methods[string(step.StepID)] = schemaObject([]string{"capability", "summary"}, map[string]any{
			"capability": map[string]any{"type": "string", "maxLength": 128, "pattern": "^[a-z0-9_.@-]*$"}, "summary": schemaString(),
		})
	}
	transitions := make([]string, len(node.OutgoingTransitions))
	for index, transition := range node.OutgoingTransitions {
		transitions[index] = string(transition.TransitionID)
	}
	return schemaObject([]string{"transition_id", "summary", "reason", "artifacts", "method_results", "node_result"}, map[string]any{
		"transition_id": map[string]any{"type": "string", "enum": transitions}, "summary": schemaString(),
		"reason":    map[string]any{"type": "string", "maxLength": domain.MaxReasonBytes},
		"artifacts": schemaObject(requiredArtifacts, artifacts), "method_results": schemaObject(requiredMethods, methods), "node_result": result,
	}), nil
}

// CurrentSubmissionSchema exposes only decisions needed by the current Action.
func CurrentSubmissionSchema(action domain.ProcessAction, blocker *domain.ProcessBlocker) (map[string]any, error) {
	if action.Validate() != nil {
		return nil, domain.ErrInvalidArgument
	}
	if action.Kind != domain.ActionResolveBlocker {
		return ActionSubmissionSchema(action.Kind)
	}
	if blocker == nil {
		return nil, domain.ErrInvalidArgument
	}
	switch blocker.Cause {
	case domain.BlockerCauseFileScopeDecision:
		return schemaObject([]string{"choice", "reason"}, map[string]any{"choice": schemaEnum("allow_once", "expand_scope", "reject"), "reason": schemaString()}), nil
	case domain.BlockerCauseWorkspaceHistoryConflict:
		return schemaObject([]string{"history_resolution"}, map[string]any{"history_resolution": schemaObject([]string{"choice", "reason"}, map[string]any{"choice": schemaEnum("accept_current_history"), "reason": schemaString()})}), nil
	case domain.BlockerCauseTaskBranchRenamePending:
		return schemaObject([]string{"rename_id", "rename_choice"}, map[string]any{"rename_id": map[string]any{"const": string(blocker.Condition.RenameID)}, "rename_choice": schemaEnum("complete", "cancel")}), nil
	case domain.BlockerCauseTaskRelocationPending:
		return schemaObject([]string{"relocation_id", "relocation_destinations"}, map[string]any{
			"relocation_id":           map[string]any{"const": string(blocker.Condition.RelocationID)},
			"relocation_destinations": map[string]any{"type": "array", "minItems": 1, "maxItems": domain.MaxRepositoryScopeEntries, "items": schemaObject([]string{"key", "repository_path"}, map[string]any{"key": schemaID(), "repository_path": schemaString()})},
		}), nil
	default:
		return schemaObject([]string{}, map[string]any{}), nil
	}
}

func ValidateCurrentSubmission(action domain.ProcessAction, blocker *domain.ProcessBlocker, raw json.RawMessage) error {
	if err := ValidateSubmissionNodeResultSyntax(raw); err != nil {
		return err
	}
	schema, err := CurrentSubmissionSchema(action, blocker)
	if err != nil {
		return err
	}
	violations := RequestStructureViolations("payload", raw, schema)
	if len(violations) != 0 {
		return domain.InvalidArgumentViolations(violations...)
	}
	return nil
}
