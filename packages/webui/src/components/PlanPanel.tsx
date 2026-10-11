import type { View } from '../lib/api';

export function PlanPanel({view, busy, confirm}: {view: View; busy: boolean; confirm: () => void}) {
  const zh = navigator.language.startsWith('zh');
  const label = (en: string, cn: string) => zh ? cn : en;
  const {plan, authorization, approval} = view.task;
  return <section>
    <h2>{label('Understanding and implementation plan', '需求理解与实施方案')}</h2>
    <p>{approval ? label('This exact plan has been confirmed.', '当前这份方案已确认。') : label('Review the complete plan before approving implementation.', '请先审阅完整方案，确认后才允许实施。')}</p>
    {view.task.repositories.map(r => <p key={r.key}><strong>{r.key}</strong> · <code>{r.workspace_origin.canonical_worktree_root}</code> · {r.workspace_origin.task_branch}</p>)}
    <dl className="plan-details">
      <dt>{label('Understanding', '需求理解')}</dt><dd>{plan.understanding}</dd>
      <dt>{label('Root cause', '根本原因')}</dt><dd>{plan.root_cause}</dd>
      <dt>{label('Approach', '实施方案')}</dt><dd>{plan.approach}</dd>
      <dt>{label('Simpler alternative', '更简单方案的比较')}</dt><dd>{plan.simpler_alternative}</dd>
      <dt>{label('Verification necessity', '验证必要性')}</dt><dd>{plan.verification_basis}</dd>
      <dt>{label('Completion and stopping', '完成与停止条件')}</dt><dd>{plan.completion}</dd>
    </dl>
    {plan.work.map(w => <div className="goal" key={w.id}><strong>{w.change}</strong><p>{label('Owner: ', '责任归属：')}{w.owner}</p><p>{w.necessity}</p><small>{label('Goals: ', '对应目标：')}{w.goal_ids.join(', ')}</small></div>)}
    <p>{label('Allowed paths: ', '允许改动范围：')}{authorization.allowed_paths?.join(', ') || label('Bound repositories', '已绑定仓库')}</p>
    <p>{label('Forbidden paths: ', '禁止改动范围：')}{authorization.forbidden_paths?.join(', ') || label('None declared', '未声明')}</p>
    <p>{label('Predicted paths (not permission): ', '预计路径（不代表授权）：')}{authorization.expected_paths?.join(', ') || '—'}</p>
    <p>{label('Maximum executions / execution time: ', '执行次数 / 时间上限：')}{authorization.limits.runs} / {authorization.limits.execution_ms} ms</p>
    <details><summary>{label('Checks, constraints and complete approved limits', '检查、约束与完整投入上限')}</summary>
      <pre>{JSON.stringify({authorization, checks:view.task.checks}, null, 2)}</pre>
    </details>
    {approval && <p>{approval.response}</p>}
    {view.task.status === 'active' && !approval && <button disabled={busy} onClick={confirm}>{label('I understand and approve this plan', '我已理解并确认此方案')}</button>}
    <small>{label('Material changes need confirmation again. Ordinary progress does not.', '实质修改方案需要重新确认；普通进度更新无需重复确认。')}</small>
  </section>;
}
