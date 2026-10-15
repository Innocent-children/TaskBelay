import { mkdtemp, mkdir, writeFile, readFile, rm, realpath } from 'node:fs/promises';
import { request } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve, delimiter } from 'node:path';
import { execFile, spawn } from 'node:child_process';
import { exampleSchema, mergeSchema, inputObject, field } from './interface-schema.mjs';

const root=resolve(import.meta.dirname,'..');
const core=resolve(process.env.ENGINEERING_CORE??join(root,'.evaluation',process.platform==='win32'?'taskbelay.exe':'taskbelay'));
const parent=await realpath(await mkdtemp(join(tmpdir(),'taskbelay-runtime-contracts-')));
const data=join(parent,'data');await mkdir(data);
const notes=JSON.parse(await readFile(join(root,'contracts/field-notes.json'),'utf8'));
const environment={...process.env,HOME:parent,USERPROFILE:parent,TASKBELAY_DATA_DIR:data};
const records=[];
const identities=new Map();
function normal(value,key='') {
  if(Array.isArray(value))return value.map(x=>normal(x,key));
  if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).map(([k,v])=>[k,normal(v,k)]));
  if(typeof value==='number'&&['pid','core_ms','calls','execution_ms','elapsed_ms'].includes(key))return 1;
  if(typeof value!=='string')return value;
  value=value.replaceAll(parent,'/example').replaceAll(root,'/package/taskbelay').replaceAll(/http:\/\/127\.0\.0\.1:\d+/g,'http://127.0.0.1:41731');
  if(key==='csrf')return 'example-session-token';
  if(/_at$|^at$/.test(key)&&/^\d{4}-/.test(value))return '2026-10-14T09:00:00Z';
  if(/^[a-f0-9]{64}$/.test(value))return 'a'.repeat(64);
  if(['task_id','request_id'].includes(key)&&value!=='example-open'&&value!=='example-cancel'){
    if(!identities.has(value))identities.set(value,'example-'+key+'-'+identities.size);
    return identities.get(value);
  }
  return value;
}
async function command(argv,input,env=environment) {
  return await new Promise(resolveResult=>{
    const child=execFile(core,argv,{env,encoding:'utf8',maxBuffer:16*1024*1024,timeout:15000},(error,stdout,stderr)=>{
      let value=stdout;try{value=JSON.parse(stdout)}catch{}
      resolveResult({exit_code:error?.code??0,stdout:value,stderr});
    });child.stdin.end(input===undefined?'':JSON.stringify(input));
  });
}
const argvSchema=inputObject({argv:field('Exact arguments after the Core executable; use separate argv entries.','array',{items:field('One literal argument.')}),stdin:field('Optional single UTF-8 JSON object; only config validate consumes it here.','object',{additionalProperties:true})},['stdin']);
const ex=(name,input,output,preconditions='Actual native Core/HTTP execution in an isolated data directory. Paths, identities, time, counters and port are normalized. The OS browser opener is replaced by a fixture; no browser was opened.')=>({name,input,output,preconditions});
function add(name,description,input_schema,examples,transport) {
  records.push({name,description,transport,input_schema,output_schema:examples.filter(x=>x.name==='success').reduce((s,e)=>mergeSchema(s,exampleSchema(e.output,notes)),undefined),examples:normal(examples)});
}
async function http(origin,method,path,body,wrongHost=false) {
  const headers={...(body?{'Content-Type':'application/json',Origin:origin}:{}),...(wrongHost?{Host:'localhost'}:{})};
  return await new Promise((yes,no)=>{
    const outgoing=request(origin+path,{method,headers},response=>{
      let text='';response.on('data',chunk=>{text+=chunk});response.on('end',()=>{
        let value=text;try{value=JSON.parse(text)}catch{}
        yes({status_code:response.statusCode,body:value});
      });
    });outgoing.on('error',no);outgoing.end(body?JSON.stringify(body):undefined);
  });
}
try {
  const coreNames=Object.keys(JSON.parse(await readFile(join(root,'internal/protocol/examples.json'),'utf8')));
  const methods=[];
  function collect(schema) {
    for(const [key,value] of Object.entries(schema?.properties??{})) {if(value.description)notes[key]=value.description;collect(value)}
    if(schema?.items)collect(schema.items);
    for(const branch of [...(schema?.anyOf??[]),...(schema?.oneOf??[])])collect(branch);
  }
  for(const name of coreNames) {
    const argv=name==='check.run'?['check','run']:name.startsWith('host-check.')?['host-check',name.slice(11)]:['call',name];
    const result=await command([...argv,'--help','--json']);if(result.exit_code!==0)throw new Error(JSON.stringify(result));
    methods.push(result.stdout);collect(result.stdout.input_schema);collect(result.stdout.output_schema);
  }
  await writeFile(join(root,'contracts/core-methods.json'),JSON.stringify(methods,null,2)+'\n');
  for(const op of ['version','help']) add('core.'+op,op==='version'?'Print the bundled Core version; no Task storage is opened.':'Print the actual native entrypoints. Method --help --json provides schemas and examples without opening Task storage.',argvSchema,[ex('success',{argv:[op]},await command([op])),ex('invalid_argument',{argv:[op,'--unexpected']},await command([op,'--unexpected']))],'CLI stdout text; nonzero exit and stderr indicate invalid arguments.');
  const config=await command(['config','validate'],{}),badConfig=await command(['config','validate'],{unexpected:true});
  const preference=inputObject({codebase_memory:field('Enable optional existing codebase index use; false by default. Does not install an index.','boolean',{default:false})},['codebase_memory']);
  add('core.config.validate','Validate one closed UTF-8 JSON configuration object, at most 16 KiB. Omitted Host preferences default false. No file, Git or Task store is opened.',inputObject(Object.fromEntries(['codex','deepseek','claude','zcode'].map(h=>[h,{...preference,description:h+' preferences.'}])),['codex','deepseek','claude','zcode']),[ex('success',{},config),ex('unknown_member',{unexpected:true},badConfig)],'One JSON object on stdin. JSON stdout is {ok,result} or {ok:false,error}; exit 0/1.');
  const bin=join(parent,'bin');await mkdir(bin);
  for(const name of ['open','xdg-open'])await writeFile(join(bin,name),'#!/bin/sh\nexit 0\n',{mode:0o755});
  environment.PATH=bin+delimiter+process.env.PATH;
  const started=await command(['webui','start','--no-open','--json']);
  if(started.exit_code!==0)throw new Error(JSON.stringify(started));
  const origin=started.stdout.url;
  for(const op of ['start','open','status']) {
    const argv=['webui',op,...(op==='start'?['--no-open']:[]),'--json'];
    const good=op==='start'?started:await command(argv);
    const bad=await command(argv,undefined,{...environment,TASKBELAY_DATA_DIR:''});
    add('core.webui.'+op,({start:'Start or reuse the loopback service. --no-open suppresses the OS browser request.',open:'Read the service address and ask the OS browser to open it; this does not prove the page rendered.',status:'Read current local service identity and readiness without starting it; a stale owned runtime receipt may be removed.'})[op]+' readiness is ready/read_only/incompatible/unavailable; URL may be empty or retain the stopped service address; readiness, not URL presence, determines availability. TASKBELAY_DATA_DIR must identify the selected local data directory.',argvSchema,[ex('success',{argv},good),ex('missing_data_directory',{argv,environment:{TASKBELAY_DATA_DIR:''}},bad)],'CLI JSON stdout RuntimeState; failures exit nonzero with stderr. --plain currently returns the same RuntimeState JSON.');
  }
  const page=await (await fetch(origin)).text();
  const csrf=page.match(/name="taskbelay-session" content="([^"]+)"/)?.[1];if(!csrf)throw new Error('Missing actual local session token');
  const repo=join(parent,'repo');await mkdir(repo);
  for(const args of [['init','-b','main'],['config','user.name','Fixture'],['config','user.email','fixture@localhost'],['commit','--allow-empty','-m','fixture']])await new Promise((yes,no)=>execFile('git',['-C',repo,...args],{env:environment},e=>e?no(e):yes()));
  const open=await command(['call','taskbelay_open_task'],{operation_id:'example-open',host:'cli',repositories:[{key:'primary',repository_path:repo}],authorization:{user_ref:'fixture-request',request:'Inspect current tracked contents',goals:[{id:'inspect',statement:'Tracked contents are inspected'}]},plan:{approach:'Inspect existing Git contents',verification_basis:'Native Git fixture',checks:[{id:'check',kind:'command',goal_ids:['inspect'],argv:['git','diff','--quiet'],rationale:'Inspect tracked contents',conditions:'Git fixture',retry:'repeatable',timeout_ms:3000}]}});
  if(open.stdout.ok!==true)throw new Error(JSON.stringify(open));
  const identity=open.stdout.result.identity;
  const query=inputObject({page:field('One-based page, default 1; 20 summaries per page.','integer',{minimum:1,maximum:1000000}),include_archived:field('Include archived terminal tasks; default false.','boolean',{default:false}),status:field('Saved Task status.','string',{enum:['active','done','cancelled']}),lifecycle:field('Core presentation filter; blocked means active with a blocker.','string',{enum:['active','blocked','done','cancelled']}),host:field('Owning Host filter.','string',{enum:['cli','codex','claude','zcode','deepseek']}),search:field('Text search of saved request/Task ID.')},['page','include_archived','status','lifecycle','host','search']);
  const definitions=[
    ['GET /api/system/status','GET','/api/system/status',undefined,inputObject({}),'Read exact loopback service readiness, Core version, data directory digest and address. No Task mutation.'],
    ['GET /api/tasks','GET','/api/tasks',undefined,query,'List saved Core summaries after filtering and before pagination. This does not refresh Git or prove applicability.'],
    ['GET /api/tasks/{task_id}','GET','/api/tasks/'+identity.task_id,undefined,inputObject({task_id:field('Actual Task ID in the path.'),refresh:field('Query string 1 refreshes actual conditions; omitted reads saved state.','string',{enum:['1','0'],default:'0'})},['refresh']),'Read the public full Task result, same Core-owned summary and service readiness. result.identity contains the next actual revision.'],
    ['GET /api/dashboard','GET','/api/dashboard',undefined,inputObject({}),'Read saved status counts and up to 20 recent Core summaries. No completion is inferred by the HTTP layer.'],
    ['POST /api/core/{tool}','POST','/api/core/taskbelay_server_info',{csrf,arguments:{}},inputObject({csrf:field('Current page taskbelay-session meta value; use same exact Origin.'),arguments:field('Exact request for the selected method from Core --help --json; native check.run is unavailable over HTTP.','object',{additionalProperties:true})}),'Dispatch one currently registered Core method. tool path must be discovered from server_info. Same method schemas, operation IDs, errors and recovery rules as CLI/MCP.'],
  ];
  for(const [name,method,path,body,schema,description] of definitions) {
    const success=await http(origin,method,path,body);
    const failure=await http(origin,method,path,body,true);
    if(success.status_code!==200||failure.status_code!==403)throw new Error(name+': '+JSON.stringify({success,failure}));
    add(name,description,schema,[ex('success',{method,path,...(body?{body}: {})},success),ex('wrong_host',{method,path,headers:{Host:'localhost'},...(body?{body}:{})},failure)],'Loopback HTTP JSON; exact Host required. POST also requires current Origin/session token. Error body uses the shared Core error envelope, with HTTP 400/403/404/409/503/500. Read outcome before retrying.');
  }
  const cancelled=await command(['call','taskbelay_cancel_task'],{...identity,operation_id:'example-cancel',reason:'End the isolated example without executing a check'});
  const revision=cancelled.stdout.result.identity.revision;
  const purgePath='/api/tasks/'+identity.task_id+'/purge';
  const purge={csrf,revision,confirm_task_id:identity.task_id};
  const wrong={...purge,confirm_task_id:'wrong-task'};
  const failedPurge=await http(origin,'POST',purgePath,wrong);
  const goodPurge=await http(origin,'POST',purgePath,purge);
  add('POST /api/tasks/{task_id}/purge','Permanently delete the exact terminal Task and its owned records after explicit confirmation. It never deletes workspaces. Active tasks or revision mismatch fail. Response loss requires reading the original identity; never select a different Task automatically.',inputObject({csrf:field('Current local session token.'),revision:field('Current terminal revision.','integer',{minimum:1}),confirm_task_id:field('Exact Task ID matching the URL; this is a destructive confirmation.')}),[ex('success',{method:'POST',path:purgePath,body:purge},goodPurge),ex('wrong_confirmation',{method:'POST',path:purgePath,body:wrong},failedPurge)],'Same exact loopback Host/Origin/session requirements as POST /api/core. Success is {ok:true}; failed confirmation has no purge effect.');
  const stopArgs=['webui','stop','--json'];
  add('core.webui.stop','Stop only the matching local service after verifying its retained identity. Tasks and files remain. An unavailable service is a successful no-op.',argvSchema,[ex('success',{argv:stopArgs},await command(stopArgs)),ex('missing_data_directory',{argv:stopArgs,environment:{TASKBELAY_DATA_DIR:''}},await command(stopArgs,undefined,{...environment,TASKBELAY_DATA_DIR:''}))],'RuntimeState JSON on success; nonzero exit/stderr on failure.');
  const initialize={jsonrpc:'2.0',id:1,method:'initialize',params:{protocolVersion:'2025-03-26',capabilities:{},clientInfo:{name:'contract-fixture',version:'1'}}};
  const response=await new Promise((yes,no)=>{
    const child=spawn(core,['mcp','--stdio'],{env:environment,stdio:['pipe','pipe','pipe']});let text='';
    const timer=setTimeout(()=>{child.kill();no(new Error('initialize timeout'))},10000);
    child.on('error',no);child.stdout.on('data',chunk=>{text+=chunk;if(text.includes('\n')){clearTimeout(timer);child.stdin.end();yes(JSON.parse(text.split('\n')[0]))}});
    child.stdin.write(JSON.stringify(initialize)+'\n');
  });
  add('core.mcp','Start the SDK stdio server; stdout is newline-delimited JSON-RPC only. Send initialize before tools/list/call. Native execution is absent. Default text is full JSON; --result-text summary is only for a Host that consumes structuredContent, which always remains complete.',argvSchema,[ex('success',{argv:['mcp','--stdio'],stdin:initialize},response),ex('missing_data_directory',{argv:['mcp','--stdio'],environment:{TASKBELAY_DATA_DIR:''}},await command(['mcp','--stdio'],undefined,{...environment,TASKBELAY_DATA_DIR:''}))],'MCP SDK transport, not a single Core Envelope process. Startup failures exit nonzero with stderr; tool results carry content, structuredContent and isError.');
  const success=name=>methods.find(m=>m.name===name).output_schema.oneOf[0].properties.result;
  const state=success('taskbelay_get_task'),pageSchema=success('taskbelay_list_tasks'),summary=pageSchema.properties.items.items;
  const bool=field('True for a successful HTTP read.','boolean',{const:true});
  const readiness=field('Actual local service readiness.','string',{enum:['ready','read_only','incompatible','unavailable']});
  const bodySchemas={
    'GET /api/tasks':inputObject({ok:bool,readiness,items:pageSchema.properties.items,page:pageSchema.properties.page,has_next:pageSchema.properties.has_next}),
    'GET /api/tasks/{task_id}':inputObject({ok:bool,readiness,summary:{...summary,description:'Same saved Core Task summary used by lists.'},result:{...state,description:'The current public get_task result; context and details are present for this HTTP endpoint.'}}),
    'GET /api/dashboard':inputObject({ok:bool,readiness,counts:{type:'object',description:'Counts by saved active/done/cancelled status.',additionalProperties:{type:'integer'}},recent:{type:'array',description:'At most 20 recent saved Core summaries.',items:summary}}),
    'POST /api/core/{tool}':{type:'object',description:'Exact output_schema from the selected Core method. Discover that method through server_info.contracts or call <tool> --help --json; includes its full error/outcome/recovery contract.',additionalProperties:true},
  };
  for(const record of records) {
    if(bodySchemas[record.name])record.output_schema.properties.body={...bodySchemas[record.name],description:bodySchemas[record.name].description??"Complete successful HTTP response body. Use the shared error branch for failures."};
  }
  await writeFile(join(root,'contracts/runtime-examples.json'),JSON.stringify(records,null,2)+'\n');
  process.stdout.write(`Exported ${records.length} runtime/HTTP contracts from actual native processes.\n`);
} finally {
  await command(['webui','stop','--json']);
  await rm(parent,{recursive:true,force:true});
}
