package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func toolRequestStructureViolations(tool string, raw []byte) []domain.ContractViolation {
	for _, definition := range catalog {
		if definition.Name != tool {
			continue
		}
		var schema map[string]any
		_ = json.Unmarshal(definition.InputSchema, &schema)
		properties := schema["properties"].(map[string]any)
		if _, submission := submissionKindForTool(tool); submission {
			properties["node_result"] = map[string]any{}
			artifacts := properties["artifacts"].(map[string]any)["properties"].(map[string]any)
			if _, declared := artifacts["current"]; !declared {
				artifacts["current"] = map[string]any{"type": "array"}
			}
		}
		if probe, ok := properties["operation_probe"].(map[string]any); ok {
			// The published recovery probe omits this repeated declaration to fit
			// the Host projection budget. Runtime input validation remains exact.
			probe["required"] = []string{"operation_id", "action_id", "payload"}
			probe["properties"].(map[string]any)["payload"] = map[string]any{}
		}
		return workflow.RequestStructureViolations("", raw, schema)
	}
	return nil
}

func hostViolations(host domain.Host) []domain.ContractViolation {
	if host.IsValid() {
		return nil
	}
	return []domain.ContractViolation{domain.ExplainedViolation("host", domain.RuleEnumValueInvalid, "host must be codex, deepseek, claude or zcode")}
}

func idViolations(path string, id domain.ID) []domain.ContractViolation {
	if id.IsValid() {
		return nil
	}
	return []domain.ContractViolation{domain.Violation(path, domain.RuleIdentifierInvalid)}
}

func revisionViolations(revision uint64) []domain.ContractViolation {
	if revision != 0 {
		return nil
	}
	return []domain.ContractViolation{domain.ExplainedViolation("revision", domain.RuleValueRange, "revision must be an integer from 1 to 18446744073709551615")}
}

func textViolations(path, value string, limit int, required bool) []domain.ContractViolation {
	if utf8.ValidString(value) && (!required || value != "") && strings.TrimSpace(value) == value && len(value) <= limit {
		return nil
	}
	presence := ""
	if required {
		presence = "non-empty, "
	}
	return []domain.ContractViolation{domain.ExplainedViolation(path, domain.RuleTextNotNormalized, fmt.Sprintf("text must be %strimmed UTF-8 and at most %d bytes", presence, limit))}
}

func requestListViolations(path string, values []string) []domain.ContractViolation {
	var out []domain.ContractViolation
	if len(values) > domain.MaxBoundedStringListItems {
		out = append(out, domain.ExplainedViolation(path, domain.RuleStringListTooLong, fmt.Sprintf("at most %d items are allowed", domain.MaxBoundedStringListItems)))
	}
	seen := map[string]bool{}
	for index, value := range values {
		child := fmt.Sprintf("%s[%d]", path, index)
		out = append(out, textViolations(child, value, domain.MaxEvidenceSummaryBytes, true)...)
		if seen[value] {
			out = append(out, domain.Violation(child, domain.RuleStringListDuplicate))
		}
		seen[value] = true
	}
	return out
}

func repositoryPathViolations(path, value string) []domain.ContractViolation {
	if validRepositoryPath(value) {
		return nil
	}
	return []domain.ContractViolation{domain.ExplainedViolation(path, domain.RuleRepositoryPathInvalid, "repository_path must be non-empty UTF-8 text of at most 4096 bytes")}
}

func workspaceOriginViolations(path string, value workspaceOriginWire) []domain.ContractViolation {
	var out []domain.ContractViolation
	if value.CarryChanges == nil {
		out = append(out, domain.Violation(path+".carry_changes", domain.RuleRequiredMemberMissing))
	}
	if value.RemoteName == nil {
		out = append(out, domain.Violation(path+".remote_name", domain.RuleRequiredMemberMissing))
	}
	if len(out) != 0 {
		return out
	}
	return repository.WorkspaceOriginViolations(path, repository.WorkspaceOriginSelection{Mode: value.Mode, SourceType: value.SourceType, CarryChanges: *value.CarryChanges, RemoteName: *value.RemoteName, BaseBranch: value.BaseBranch, BaseCommit: value.BaseCommit, TaskBranch: value.TaskBranch, ProvisioningReceiptID: value.ProvisioningReceiptID})
}

func operationProbeViolations(probe *operationProbeWire) []domain.ContractViolation {
	if probe == nil {
		return nil
	}
	var out []domain.ContractViolation
	for _, value := range []struct {
		name string
		id   domain.ID
	}{{"operation_id", probe.OperationID}, {"action_id", probe.ActionID}} {
		out = append(out, idViolations("operation_probe."+value.name, value.id)...)
	}
	definition := workflow.StandardProcess()
	if probe.ProcessID != definition.Reference.ID {
		out = append(out, domain.ExplainedViolation("operation_probe.process_id", domain.RuleCurrentValueRequired, "process_id must identify the supported standard-development process"))
	}
	for _, value := range []struct {
		name   string
		digest domain.Digest
	}{
		{"process_definition_digest", probe.ProcessDefinitionDigest}, {"repository_binding_digest", probe.RepositoryBindingDigest},
		{"issuance_identity_digest", probe.IssuanceIdentityDigest}, {"issuance_history_digest", probe.IssuanceHistoryDigest}, {"issuance_content_digest", probe.IssuanceContentDigest},
	} {
		if !value.digest.IsValid() {
			out = append(out, domain.ExplainedViolation("operation_probe."+value.name, domain.RuleValueFormat, "digest must contain exactly 64 lowercase hexadecimal characters"))
		}
	}
	if probe.ProcessDefinitionDigest.IsValid() && probe.ProcessDefinitionDigest != definition.Reference.DefinitionDigest {
		out = append(out, domain.Violation("operation_probe.process_definition_digest", domain.RuleCurrentValueRequired))
	}
	if probe.ExpectedRevision == 0 {
		out = append(out, domain.ExplainedViolation("operation_probe.expected_revision", domain.RuleValueRange, "expected_revision must be at least 1"))
	}
	node, err := workflow.NodeDefinition(definition, probe.SourceCursor)
	if err != nil {
		out = append(out, domain.ExplainedViolation("operation_probe.source_cursor", domain.RuleEnumValueInvalid, "source_cursor must name a node in the current process definition"))
	} else if node.ActionKind != probe.ActionKind {
		out = append(out, domain.ExplainedViolation("operation_probe.action_kind", domain.RuleActionKindPayloadMismatch, "action_kind must match the action of source_cursor"))
	}
	if len(probe.Payload) == 0 {
		out = append(out, domain.Violation("operation_probe.payload", domain.RuleRequiredMemberMissing))
	} else if len(out) == 0 && !bytes.Equal(bytes.TrimSpace(probe.Payload), []byte("null")) {
		var err error
		if probe.SourceCursor == domain.NodeBlocked {
			_, _, err = recovery.DecodeBlockerResolutionPayload(probe.Payload)
		} else {
			err = workflow.ValidateRetainedPayload(probe.SourceCursor, probe.Payload)
		}
		if err != nil {
			var failure *domain.Error
			if errors.As(err, &failure) && failure != nil && len(failure.Violations) != 0 {
				for _, violation := range failure.Violations {
					violation.Path = "operation_probe." + violation.Path
					out = append(out, violation)
				}
			} else {
				message := "payload must be null or the complete saved payload of source_cursor, with its original transition, artifact and method records"
				if failure != nil && failure.PublicExplanation() != "" {
					message = failure.PublicExplanation()
				}
				out = append(out, domain.ExplainedViolation("operation_probe.payload", domain.RuleMemberDependency, message))
			}
		}
	}
	return out
}
