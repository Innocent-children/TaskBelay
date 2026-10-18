package application

import (
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
)

// OriginInput includes only Host-owned selections; Core fills observed identity.
type OriginInput struct {
	Mode                  domain.WorkspaceMode `json:"mode" description:"current_branch/new_branch/dedicated_worktree。"`
	SourceType            string               `json:"source_type" description:"local/remote，说明冻结来源。"`
	CarryChanges          bool                 `json:"carry_changes" description:"是否按原选择携带本地变动，remote来源不得为true。"`
	RemoteName            string               `json:"remote_name" description:"remote来源的真实远端名；local来源为空。"`
	BaseBranch            string               `json:"base_branch" description:"原来源分支，不自行换成main。"`
	BaseCommit            string               `json:"base_commit" description:"冻结的精确来源提交。"`
	TaskBranch            string               `json:"task_branch" description:"本任务实际目标分支。"`
	ProvisioningReceiptID domain.ID            `json:"provisioning_receipt_id" description:"Core绑定的原准备收据身份，用于确认清理归属。"`
}

func (o OriginInput) selection() repository.WorkspaceOriginSelection {
	return repository.WorkspaceOriginSelection{Mode: o.Mode, SourceType: o.SourceType, CarryChanges: o.CarryChanges, RemoteName: o.RemoteName, BaseBranch: o.BaseBranch, BaseCommit: o.BaseCommit, TaskBranch: o.TaskBranch, ProvisioningReceiptID: o.ProvisioningReceiptID}
}
