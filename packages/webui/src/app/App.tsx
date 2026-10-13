import { useEffect, useRef, useState } from 'react';
import {BudgetPanel,type BudgetDecision} from '../components/BudgetPanel';
import { PlanPanel } from '../components/PlanPanel';
import { call, callTask, identity, read, type Summary, type View } from '../lib/api';

const zh = navigator.language.startsWith('zh');
const label = (en: string, cn: string) => zh ? cn : en;
const duration = (ms: number) => ms < 1000 ? ms + ' ms' : (ms / 1000).toFixed(1) + ' s';

export function App() {
  const detailRequest = useRef(0);
  const listRequest = useRef(0);
  const [tasks, setTasks] = useState<Summary[]>([]);
  const [view, setView] = useState<View | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [log, setLog] = useState('');
  const [page, setPage] = useState(1);
  const [hasNext, setHasNext] = useState(false);
  const [includeArchived, setIncludeArchived] = useState(false);

  async function load(requestedPage = page, archived = includeArchived) {
    const request=++listRequest.current;
    try {
      const data = await read<{items: Summary[];page:number;has_next:boolean}>(`/api/tasks?page=${requestedPage}&include_archived=${archived}`);
      if(request !== listRequest.current) return;
      setTasks(data.items); setPage(data.page); setHasNext(data.has_next); setIncludeArchived(archived);
    } catch (e) { setError(String(e)); }
  }
  useEffect(() => {
    void load(1, false);
    function navigate() {
      const match = /^\/tasks\/([^/]+)\/?$/.exec(window.location.pathname);
      if (match) { try { void detail(decodeURIComponent(match[1])); } catch { setError('Invalid task address'); } }
      else { detailRequest.current++; setView(null); setBusy(false); }
    }
    navigate(); window.addEventListener('popstate', navigate);
    return () => window.removeEventListener('popstate', navigate);
  }, []);
  function select(id:string) {
    window.history.pushState(null, '', '/tasks/' + encodeURIComponent(id));
    void detail(id);
  }
  async function updateBudget(budget:BudgetDecision,response:string) {
    if(!view)return;setBusy(true);setError('');
    try {setView(await callTask('taskbelay_update_budget',{...identity(view),authorization_digest:view.authorization_digest,budget,user_ref:'local-webui-budget-decision',response}));void load();}
    catch(e){setError(String(e));}finally{setBusy(false);}
  }
  async function archive() {
    if (!view) return;
    setBusy(true); setError('');
    try { setView(await callTask('taskbelay_archive_task', {...identity(view), archived:!view.task.archived})); void load(); }
    catch(e) { setError(String(e)); } finally { setBusy(false); }
  }

  async function detail(id: string, refresh = false) {
    const request=++detailRequest.current;
    setBusy(true); setError(''); setLog('');
    try { const next=await read<{result: View}>('/api/tasks/' + encodeURIComponent(id) + (refresh ? '?refresh=1' : '')); if(request===detailRequest.current)setView(next.result); }
    catch (e) { if(request===detailRequest.current)setError(String(e)); }
    finally { if(request===detailRequest.current)setBusy(false); }
  }
  async function human(check: string, passed: boolean) {
    if (!view) return;
    const summary = window.prompt(label('Describe what you actually checked and observed.', '说明你实际检查的内容与观察结果。'));
    if (!summary) return;
    setBusy(true); setError('');
    try {
      setView(await callTask('taskbelay_record_evidence', {
        ...identity(view), check_id: check, source: 'human', status: passed ? 'passed' : 'failed',
        summary, user_ref: 'local-webui-user-observation', content_digest: view.content_digest,
      }));
    } catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }
  async function cancel() {
    if (!view) return;
    const reason = window.prompt(label('Reason to cancel this task (files remain).', '取消此任务的原因（保留文件）。'));
    if (!reason) return;
    setBusy(true); setError('');
    try { setView(await callTask('taskbelay_cancel_task', {...identity(view), reason})); void load(); }
    catch (e) { setError(String(e)); }
    finally { setBusy(false); }
  }
  async function runLog(id: string) {
    try {
      const v = await call<{status: string; stdout: string; stderr: string; truncated: boolean}>('taskbelay_get_run', {run_id: id});
      setLog(v.status + (v.truncated ? ' · ' + label('truncated output', '输出已截断') : '') + '\n' + v.stdout + '\n' + v.stderr);
    } catch (e) { setError(String(e)); }
  }

  return <div className="shell">
    <header><a href="/">TaskBelay</a><span>{label('Development progress and results', '开发进展与检查结果')}</span>
      <button disabled={busy} onClick={() => view ? void detail(view.task.task_id, true) : void load()}>{label('Refresh actual state', '刷新实际状态')}</button>
    </header>
    <main><aside><h2>{label('Your tasks', '你的任务')}</h2>
      {tasks.map(t => <button className={'task ' + (view?.task.task_id === t.task_id ? 'selected' : '')} key={t.task_id} onClick={() => select(t.task_id)}>
        <strong>{t.request_summary}</strong><span>{t.lifecycle}</span><small>{t.worktree_path}</small>
      </button>)}
      <div className="pagination"><button disabled={busy || page <= 1} onClick={() => void load(page-1)}>{label('Previous', '上一页')}</button><span>{page}</span><button disabled={busy || !hasNext} onClick={() => void load(page+1)}>{label('Next', '下一页')}</button></div>
      <label><input type="checkbox" checked={includeArchived} onChange={e => { const selected=e.target.checked; setIncludeArchived(selected); void load(1,selected); }} />{label('Include archived', '包含已归档任务')}</label>
      {tasks.length === 0 && <p>{label('Start in your coding agent with the TaskBelay Skill. Existing authorization is retained.', '在编码代理中使用 TaskBelay Skill 开始；已有授权继续有效。')}</p>}
    </aside><article>
      {error && <div role="alert" className="error">{error}</div>}
      {!view ? <section><h1>{label('See results and remaining gaps', '查看结果与剩余问题')}</h1>
        <p>{label('TaskBelay keeps authorized outcomes, actual changes and execution results together. Choose a saved task to inspect it.', 'TaskBelay 将授权目标、实际改动与执行结果保存在一起。选择已保存的任务查看。')}</p>
      </section> : <>
        <section><div className="row"><span className="badge">{view.task.status}</span>
          <small>{view.refreshed ? label('Actual conditions refreshed', '已刷新实际条件') : label('Saved state · refresh to assess applicability', '保存状态 · 刷新后判断适用性')}</small>
        </div><h1>{view.task.authorization.request}</h1>{view.task.summary && <p>{view.task.summary}</p>}
          <h2>{label('Current results', '当前结果')}</h2>
          {view.task.authorization.goals.map(g => {
            const p = view.task.progress?.find(p => p.goal_id === g.id);
            return <div className="goal" key={g.id}><strong>{g.statement}</strong><span>{p?.status ?? label('planned', '待执行')}</span>{p && <p>{p.summary}</p>}</div>;
          })}
        </section>
        <section><h2>{label('Remaining problems and decisions', '剩余问题与必要决定')}</h2>
          {view.task.issue && <div className="error"><p>{view.task.issue.summary}</p><ul>{view.task.issue.options.map(x => <li key={x}>{x}</li>)}</ul>
            <p>{label('Give your decision to the coding agent; it will update the authorized contract.', '向编码代理给出决定，由它更新授权契约。')}</p>
          </div>}
          {view.decision.findings.filter(f => f.blocking).map(f => <p className="finding" key={f.id}>{f.path && <code>{f.path}</code>} {f.detail}</p>)}
          {view.decision.missing_checks.length > 0 && <p>{label('Still required: ', '仍需验证：')}{view.decision.missing_checks.join(', ')}</p>}
          {view.decision.pending_runs.map(id => <p key={id}>{label('Unresolved execution: ', '未确定执行：')}<button onClick={() => void runLog(id)}>{id}</button></p>)}
        </section>
        <PlanPanel view={view} />
        <section><h2>{label('Evidence and actual executions', '证据与实际执行')}</h2>
          <p>{label('Claims, executions and acceptance conclusions are distinguished. Evidence applies only to its declared inputs and conditions.', '声明、执行和验收结论分别记录。证据仅适用于声明的输入与条件。')}</p>
          {view.decision.accepted_failures.map(x => <div className="finding" key={x.check_id}><strong>{label('Accepted existing failures: ', '明确接受的既有失败：')}{x.check_id}</strong><p>{x.failures.join(', ')}</p><p>{x.reason}</p></div>)}
          {(view.task.evidence ?? []).map(e => {
            const f = view.decision.evidence.find(f => f.evidence_id === e.id);
            return <div className="evidence" key={e.id}><strong>{e.check_id}</strong><span>{e.source} · {e.status}</span><p>{e.summary}</p><small>{f?.reason}</small>
              {e.source === 'core_execution' && <button onClick={() => void runLog(e.id)}>{label('Read actual output', '查看实际输出')}</button>}
            </div>;
          })}
          {view.task.checks.filter(c => c.kind === 'human').map(c => <div key={c.id}><p>{c.rationale} · {c.conditions}</p>
            <button disabled={busy || !view.refreshed} onClick={() => void human(c.id, true)}>{label('I checked: passed', '我已检查：通过')}</button>{' '}
            <button disabled={busy || !view.refreshed} onClick={() => void human(c.id, false)}>{label('I checked: failed', '我已检查：失败')}</button>
          </div>)}{log && <pre>{log}</pre>}
        </section>
        <section><h2>{label('Cumulative cost', '累计投入')}</h2><div className="metrics">
          <div><strong>{view.task.usage.runs}{(view.task.authorization.limits.runs ?? 0) > 0 ? ' / '+view.task.authorization.limits.runs : ''}</strong><span>{label('command executions', '命令执行')}</span></div>
          <div><strong>{duration(view.task.usage.execution_ms)}</strong><span>{label('actual execution', '实际执行')}</span></div>
          <div><strong>{view.task.usage.core_calls} · {duration(view.task.usage.core_ms)}</strong><span>{label('Task Core calls / wall time', '任务 Core 调用 / 墙钟时间')}</span></div>
          <div><strong>{duration(view.task.usage.host_reported_ms + view.task.usage.host_reported_wait_ms)} · {view.task.usage.host_reported_tokens} tokens</strong><span>{label('Host reports · may be incomplete', '宿主报告 · 可能不完整')}</span></div>
        </div>
          <p>{label('Unknown command time, conservatively charged: ', '未确定命令时间，保守计入：')}{duration(view.task.usage.uncertain_ms)}</p>
          <p>{label('All local Core calls, including queries without a Task: ', '本地全部 Core 调用（包括未关联任务的查询）：')}{view.control_cost.calls} · {duration(view.control_cost.wall_ms)}
            {' · '}{label('unknown estimate: ', '未知成本估算：')}{duration(view.control_cost.unknown_ms)}{' · '}{label('pending calls (including this request): ', '未结算调用（包含本次请求）：')}{view.control_cost.pending}
          </p><p>{label('Core wall time includes execution and waiting; these figures overlap and must not be added. Revisions and history compaction retain cumulative cost.', 'Core 墙钟时间包含执行与等待，各项存在重叠，不能直接相加。修订与历史压缩保留累计消耗。')}</p>
        </section>
        {view.task.status==='active'&&<BudgetPanel key={view.authorization_digest} view={view} busy={busy} update={(b,r)=>void updateBudget(b,r)}/>}
        <section><h2>{label('Constraints and limits', '约束与边界')}</h2>
          {view.task.authorization.constraints?.map(c => <p key={c.id}>{c.statement}</p>)}{view.task.risks?.map(r => <p key={r}>{r}</p>)}
          <p>{label('Structured write hooks cover known paths. Shell and external edits are observed at checkpoints. Semantic conclusions remain the agent’s reasoning.', '结构化写入钩子检查已知路径；shell 与外部编辑在检查点观察。语义结论仍属于代理判断。')}</p>
          {view.task.status !== 'active' && <button disabled={busy} onClick={() => void archive()}>{view.task.archived ? label('Restore from archive','取消归档') : label('Archive task','归档任务')}</button>}
          {view.task.status === 'active' && <button disabled={busy} onClick={() => void cancel()}>{label('Cancel task', '取消任务')}</button>}
        </section>
      </>}
    </article></main>
  </div>;
}
