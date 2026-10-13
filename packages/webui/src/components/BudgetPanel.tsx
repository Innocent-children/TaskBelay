import {useState} from 'react';
import type {View} from '../lib/api';
export interface BudgetDecision {runs:number;execution_ms:number;work_ms:number;tokens:number}
export function BudgetPanel({view,busy,update}:{view:View;busy:boolean;update:(budget:BudgetDecision,response:string)=>void}) {
 const zh=navigator.language.startsWith('zh'),label=(en:string,cn:string)=>zh?cn:en,l={runs:0,execution_ms:0,core_ms:0,host_ms:0,tokens:0,...view.task.authorization.limits};
 const [runs,setRuns]=useState(l.runs),[command,setCommand]=useState(l.execution_ms/60000),[work,setWork]=useState(Math.max(l.core_ms,l.host_ms)/60000),[tokens,setTokens]=useState(l.tokens);
 const budget={runs,execution_ms:Math.round(command*60000),work_ms:Math.round(work*60000),tokens};
 const limits={runs,execution_ms:budget.execution_ms,core_ms:budget.work_ms,host_ms:budget.work_ms,tokens};
 const changes=(Object.keys(limits) as (keyof typeof limits)[]).filter(k=>limits[k]!==l[k]).map(k=>`${k}: ${l[k]} → ${limits[k]}`);
 const valid=Object.values(budget).every(n=>Number.isSafeInteger(n)&&n>=0)&&runs<=10000&&budget.execution_ms<=86400000;
 return <details className="task-details"><summary>{label('Change budget','调整预算')}</summary>
 <p>{label('Zero means no limit. Existing usage is retained.', '0 表示不设上限，已有消耗保留。')}</p>
 <p>{label('Approve only the changes below. Work time updates both Core and reported-work caps.','只批准下列变化。工作时间同时更新 Core 与报告工作时间上限。')}</p>
 <div className="budget-fields"><label>{label('Executions','执行次数')} <input type="number" min="0" value={runs} onChange={e=>setRuns(Number(e.target.value))}/></label>
 <label>{label('Command minutes','累计命令分钟')} <input type="number" min="0" step="any" value={command} onChange={e=>setCommand(Number(e.target.value))}/></label>
 <label>{label('Work minutes','累计工作分钟')} <input type="number" min="0" step="any" value={work} onChange={e=>setWork(Number(e.target.value))}/></label>
 <label>Tokens <input type="number" min="0" value={tokens} onChange={e=>setTokens(Number(e.target.value))}/></label>
 </div>
 <ul>{changes.map(c=><li key={c}>{c}</li>)}</ul>
 <button disabled={busy||!valid||!changes.length} onClick={()=>update(budget,label('I approve these exact budget changes: ','我批准这些具体预算变化：')+changes.join('; '))}>{label('Approve budget changes','批准预算变化')}</button>
 </details>;
}
