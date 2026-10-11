package application

import (
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
)

// OriginInput includes only Host-owned selections; Core fills observed identity.
type OriginInput struct {
	Mode                  domain.WorkspaceMode `json:"mode"`
	SourceType            string               `json:"source_type"`
	CarryChanges          bool                 `json:"carry_changes"`
	RemoteName            string               `json:"remote_name"`
	BaseBranch            string               `json:"base_branch"`
	BaseCommit            string               `json:"base_commit"`
	TaskBranch            string               `json:"task_branch"`
	ProvisioningReceiptID domain.ID            `json:"provisioning_receipt_id"`
}

func (o OriginInput) selection() repository.WorkspaceOriginSelection {
	return repository.WorkspaceOriginSelection{Mode: o.Mode, SourceType: o.SourceType, CarryChanges: o.CarryChanges, RemoteName: o.RemoteName, BaseBranch: o.BaseBranch, BaseCommit: o.BaseCommit, TaskBranch: o.TaskBranch, ProvisioningReceiptID: o.ProvisioningReceiptID}
}
