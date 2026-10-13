import type { View } from '../lib/api';

export function PlanPanel({view, busy, confirm}: {view: View; busy: boolean; confirm: () => void}) {
  const zh = navigator.language.startsWith('zh');
  const label = (en: string, cn: string) => zh ? cn : en;
  const {plan, authorization, approval} = view.task;
  const names:Record<string,string>={runs:label('Executions','执行次数'),execution_ms:label('Command time','命令时间'),core_ms:label('Core work time','Core 工作时间'),host_ms:label('Reported work and wait','报告的工作与等待'),tokens:'Tokens'};
  const value=(name:string,n:number)=>name.endsWith('_ms')?`${Math.round(n/1000)} s`:String(n);
  return <section>
    <h2>{label('Understanding and implementation plan', '需求理解与实施方案')}</h2>
    <p>{approval ? label('This exact plan has been confirmed.', '当前这份方案已确认。') : label('Review the complete plan before approving implementation.', '请先审阅完整方案，确认后才允许实施。')}</p>
    {view.task.repositories.map(r => <p key={r.key}><strong>{r.key}</strong> · <code>{r.workspace_origin.canonical_worktree_root}</code> · {r.workspace_origin.task_branch}{r.remote_baseline&&<small> · {label('Initial upstream: ','初始上游：')}{r.remote_baseline.state} · {r.remote_baseline.checked_at}</small>}{r.remote_latest&&<small> · {label('Latest upstream check: ','最近上游核验：')}{r.remote_latest.state} · {r.remote_latest.checked_at} · {r.remote_latest.basis}</small>}</p>)}
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
    <h3>{label('Verification and budget','验证与预算')}</h3>
    <ul>{view.task.checks.map(c=><li key={c.id}><strong>{c.id}</strong> · {c.rationale} · {c.conditions}</li>)}</ul>
    {!authorization.dependencies?.length&&<p>{label('Additional dependencies: none authorized.','新增依赖：尚未授权。')}</p>}
    {authorization.dependencies?.map(p=><p key={p.path}>{label('Approved dependencies: ','已允许的依赖：')}<code>{p.path}</code> · {p.allowed.join(', ')}</p>)}
    <table><thead><tr><th>{label('Limit','额度')}</th><th>{label('Actual','实际')}</th><th>{label('Reserved','预留')}</th><th>{label('Uncertain','未确定')}</th><th>{label('Maximum','上限')}</th><th>{label('Remaining','剩余')}</th></tr></thead><tbody>{view.budget.limits.map(b=><tr key={b.name}><td>{names[b.name]}</td><td>{value(b.name,b.actual)}</td><td>{value(b.name,b.reserved)}</td><td>{value(b.name,b.uncertain)}</td><td>{value(b.name,b.limit)}</td><td>{value(b.name,b.remaining)}</td></tr>)}</tbody></table>
    {view.budget.checks.filter(c=>c.required).map(c=><p key={c.check_id}><strong>{c.check_id}</strong> · {label('Next reservation: ','下次所需预留：')}{value('execution_ms',c.timeout_ms)} · {c.startable?label('Budget available','额度可预留'):label('Cannot reserve under current limits','当前额度无法预留')}</p>)}
    {view.budget.warnings.map(w=><p className="finding" key={w}>{w}</p>)}
    <p>{label('Aggregate timeouts are estimates. Core and reported work time overlap; do not add them.','整套超时是估计值。Core 与报告工作时间可能重叠，不能直接相加。')}</p>
    <details><summary>{label('Checks, constraints and complete approved limits', '检查、约束与完整投入上限')}</summary>
      <pre>{JSON.stringify({authorization, checks:view.task.checks}, null, 2)}</pre>
    </details>
    {approval && <p>{approval.response}</p>}
    {view.task.status === 'active' && !approval && <button disabled={busy || !view.budget.feasible} onClick={confirm}>{label('I understand and approve this plan', '我已理解并确认此方案')}</button>}
    <small>{label('Material changes need confirmation again. Ordinary progress does not.', '实质修改方案需要重新确认；普通进度更新无需重复确认。')}</small>
  </section>;
}
