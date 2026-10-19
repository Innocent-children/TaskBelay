import { EventEmitter } from "node:events";
import { mkdtemp, mkdir, readFile, writeFile, rm, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { dirname, join, resolve } from 'node:path';
import { OPERATIONS, isDataDeletion } from '../packages/taskbelay/lib/cli.mjs';
import { runMain } from '../packages/taskbelay/lib/lifecycle.mjs';
import { runPet } from '../packages/taskbelay/lib/pet.mjs';
import { exampleSchema, mergeSchema, inputObject, field } from './interface-schema.mjs';
import { runCLI as codexCLI } from '../packages/codex/bin/taskbelay-codex.mjs';
import * as zcode from '../packages/zcode/lib/lifecycle.mjs';
import * as claude from '../packages/claude/lib/lifecycle.mjs';
import { runHook as codexHook } from '../packages/codex/plugin/hooks/pre-tool-use.mjs';
import { runHook as claudeHook } from '../packages/claude/plugin/hooks/pre-tool-use.mjs';
import { runHook as zcodeHook } from '../packages/zcode/hooks/pre-tool-use.mjs';

// Only temporary owned directories are changed. External Host/native drivers are
// simulated explicitly; these examples establish serialization, not live support.
const root=resolve(import.meta.dirname,'..');
const parent=await realpath(await mkdtemp(join(tmpdir(),'taskbelay-management-contracts-')));
const notes=JSON.parse(await readFile(join(root,'contracts/field-notes.json'),'utf8'));
const version=(await readFile(join(root,'CORE_VERSION'),'utf8')).trim();
const records=[];
function normal(value,key='') {
  if(Array.isArray(value))return value.map(v=>normal(v,key));
  if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).map(([k,v])=>[k,normal(v,k)]));
  if(typeof value!=='string')return value;
  value=value.replaceAll(parent,'/example').replaceAll(root,'/package/taskbelay');
  if(/^[a-f0-9]{8}-[a-f0-9-]{27}$/.test(value))return '00000000-0000-4000-8000-000000000001';
  if(/_at$/.test(key)&&/^\d{4}-/.test(value))return '2026-10-14T09:00:00Z';
  if(/^[a-f0-9]{64}$/.test(value))return 'a'.repeat(64);
  return value;
}
function add(name,description,inputSchema,examples,transport='argv array, not a shell string. For JSON commands parse stdout and retain exit_code; stderr is a separate diagnostic stream.') {
  const success=examples.filter(e=>e.name==='success'||e.name.startsWith('success_'));
  records.push({name,description,transport,input_schema:inputSchema,output_schema:success.reduce((s,e)=>mergeSchema(s,exampleSchema(e.output,notes)),undefined),examples:normal(examples)});
}
const argvSchema=inputObject({argv:field('Exact command arguments after the executable; each option is a separate string.','array',{items:field('One literal argument.')} )});
const example=(name,input,output,preconditions='Actual serializer using isolated temporary paths and simulated external Host drivers. It does not establish a real installation, session, or native desktop action.')=>({name,input,output,preconditions});
async function captured(action) {
  let stdout='',stderr='';
  const streams={stdout:{write:x=>{stdout+=x}},stderr:{write:x=>{stderr+=x}}};
  const result=await action(streams);
  let value=stdout;try{value=JSON.parse(stdout)}catch{}
  return {exit_code:result?.code??0,stdout:value,stderr};
}
async function lifecycle() {
  const home=join(parent,'manager');await mkdir(home);
  const states={codex:'absent',deepseek:'absent',claude:'absent',zcode:'absent'};
  const driver=host=>({maintenanceTargets:async()=>({registeredCorePaths:[],installedRuntime:null}),knownProfiles:async()=>host==='deepseek'?['web']:[],resolveTargetVersion:async()=> '0.8.0',observe:async()=>({host,profile:host==='deepseek'?'web':null,hostAvailable:true,hostVersion:'1.0.0',state:states[host],packageVersion:states[host]!=='absent'?'0.8.0':null,coreVersion:null,receipt:states[host]!=='absent'?{}:null}),execute:async operation=>{states[host]=operation==='uninstall'?'absent':'ready';return {changed:true,completedSteps:[host+'.'+operation]}}});
  const dependencies={homeDirectory:home,environment:{LANG:'en'},platform:'darwin',arch:'arm64',codexDriver:driver('codex'),deepseekDriver:driver('deepseek'),claudeDriver:driver('claude'),zcodeDriver:driver('zcode'),isTTY:false};
  for(const operation of OPERATIONS) {
    let args=[operation,'--host',isDataDeletion(operation)?'all':'codex','--json',...(['status','doctor'].includes(operation)||isDataDeletion(operation)?[]:['--yes'])];
    const invoke=args=>captured(({stdout,stderr})=>runMain(args,{...dependencies,output:stdout,errorOutput:stderr}));
    let unconfirmed;
    if(isDataDeletion(operation)) {
      await mkdir(join(home,'.taskbelay/data'),{recursive:true});
      await writeFile(join(home,'.taskbelay/data','example-record'),'isolated example');
      unconfirmed=await invoke(args);
      if(unconfirmed.exit_code!==3)throw new Error('Expected real deletion confirmation');
      args=[...args,'--confirm-delete',unconfirmed.stdout.confirmation.token];
    }
    const good=await invoke(args);
    if(good.stdout?.error)throw new Error(operation+': '+JSON.stringify(good));
    const extra=[];
    if(operation==='upgrade') {
      const signalSource=new EventEmitter();
      const saved=states.codex;states.codex='partial';
      const interrupted=await captured(({stdout,stderr})=>runMain(args,{...dependencies,signalSource,output:stdout,errorOutput:stderr,
        codexDriver:{...driver('codex'),execute:async()=>{signalSource.emit('SIGINT');throw new Error('Operation interrupted during package replacement');}}}));
      states.codex=saved;
      extra.push(example('interrupted',{argv:args},interrupted,'Simulated SIGINT during a package replacement; the real serializer retains the operation ID and reports unknown changes without replaying the operation.'));
    }
    if(operation==='upgrade') {
      let launcherVersion='0.3.0';
      const selfArgs=['upgrade','--self','--yes','--json'];
      const launcherDriver={observe:async()=>({host:'taskbelay',profile:null,state:'ready',packageInstalled:true,packageVersion:launcherVersion,packageRoot:'/example/global/taskbelay',channel:'npm'}),resolveTarget:async()=>({version:'0.3.1',artifact:'taskbelay@0.3.1',channel:'npm',packageRoot:'/example/global/taskbelay'}),execute:async()=>{launcherVersion='0.3.1';return {changed:true,completedSteps:['taskbelay.upgrade']}}};
      const selfResult=await captured(({stdout,stderr})=>runMain(selfArgs,{...dependencies,output:stdout,errorOutput:stderr,launcherDriver,petInstaller:{ensurePetInstalled:async()=>({installed:false})},stopPetForCore:async()=>({stopped:false})}));
      extra.push(example('success_launcher',{argv:selfArgs},selfResult));
    }
    const bad=[operation,'--unexpected','--json'];
    add('taskbelay.lifecycle.'+operation,
      ({status:'Read current installed resources and retained operation outcomes.',doctor:'Read the same installation and return concrete diagnostics.',install:'Install the selected Host adapter under existing explicit installation authority.',upgrade:'Update the selected owned adapter to the requested target.',repair:'Restore missing or inconsistent owned registration and package resources.',reinstall:'Replace the selected owned adapter after the requested reinstall decision.',uninstall:'Remove selected owned adapter resources while retaining Task data.', 'delete-data':'Delete confirmed TaskBelay user data and reset preferences while retaining plugin files, Agent registrations and installation records. Recreate empty data directories at their original paths. --yes never approves deletion.', 'delete-all':'Remove every managed plugin and delete only the confirmed shared data, keeping the launcher and desktop application. Pending removals or unresolved executions stop cleanup. --yes never approves deletion.'})[operation]+' Repeat --host for an Agent subset, or use all; --profile is repeatable for DeepSeek. Upgrade only updates installed plugins; --self includes the global launcher and --self-package supplies a complete local distribution without changing channels. Shared data deletion covers all Agents and uses --confirm-delete plus --confirm-permanent for permanent deletion, --confirm-explicit-data for extra paths and --confirm-zcode-closed when ZCode is installed. --json returns the complete operation result; status/action_required/partial are not interchangeable with ready. Read next_steps and retained operation_id after uncertainty.',argvSchema,[example('success',{argv:args},good),example('invalid_option',{argv:bad},await invoke(bad)),...(unconfirmed?[example('confirmation_required',{argv:args.slice(0,-2)},unconfirmed)]:[]),...extra]);
  }
}
async function hostLifecycle() {
  const home=join(parent,'hosts');await mkdir(home);
  const environment={...process.env,HOME:home,USERPROFILE:home,LOCALAPPDATA:join(home,'appdata'),TASKBELAY_DATA_DIR:'',LANG:'en'};
  const packageRoot=join(parent,'zcode-package');await mkdir(join(packageRoot,'.zcode-plugin'),{recursive:true});
  const manifest={name:'taskbelay-zcode',version:'0.1.0',files:['.zcode-plugin/plugin.json','marketplace.json','payload.txt']};
  await writeFile(join(packageRoot,'package.json'),JSON.stringify(manifest));
  await writeFile(join(packageRoot,'.zcode-plugin/plugin.json'),JSON.stringify({name:manifest.name,version:manifest.version}));
  await writeFile(join(packageRoot,'marketplace.json'),JSON.stringify({name:'taskbelay-zcode-local',plugins:[{name:manifest.name,version:manifest.version,source:'./'}]}));
  await writeFile(join(packageRoot,'payload.txt'),'fixture');
  const core=async()=>({stdout:'taskbelay '+version+'\n'});
  let installed=false,market=false;
  const cm=JSON.parse(await readFile(join(root,'packages/claude/package.json'),'utf8'));
  const claudeRoot=join(parent,'claude-package');
  for(const file of ['package.json',...cm.files]) {
    const target=join(claudeRoot,file);await mkdir(dirname(target),{recursive:true});
    let bytes;try {bytes=await readFile(join(root,'packages/claude',file))}catch(error){if(error.code!=='ENOENT')throw error;if(file==='LICENSE')bytes=await readFile(join(root,'LICENSE'));else if(file.startsWith('runtime/'))bytes=Buffer.from('fixture runtime; invocation is injected');else throw error}
    await writeFile(target,bytes);
  }
  const claudeFixture=await import(pathToFileURL(join(claudeRoot,'lib/lifecycle.mjs')));
  const exec=async(_command,args)=>{
    let result;
    if(args[0]==='--version')return {stdout:'2.1.270'};
    if(args[1]==='list')result=installed?[{id:claude.pluginID,scope:'user',version:cm.version,enabled:true,installPath:claudeRoot}]:[];
    else if(args[1]==='marketplace') { if(args[2]==='list')result=market?[{name:'taskbelay-claude-local',source:'directory',path:claudeRoot}]:[];else {market=args[2]==='add';result={outcome:'ok'}} }
    else {installed=args[1]==='install';result={outcome:'ok'}}
    return {stdout:JSON.stringify(result)};
  };
  for(const [host,module,options] of [['zcode',zcode,{environment,packageRoot,core}],['claude',claudeFixture,{environment,core,exec}]]) {
    for(const operation of ['status','setup','remove']) {
      const output=await module[operation](options);
      const badOptions={...options,environment:{...environment,TASKBELAY_DATA_DIR:'relative'}};
      let failure;try{await module[operation](badOptions);throw new Error('accepted invalid path')}catch(error){failure={exit_code:1,stdout:'',stderr:`taskbelay-${host}: ${error.message}\n`}}
      const name=output.error?'unavailable':'success';
      const examples=[example(name,{argv:[operation,'--json']},output.error?{exit_code:1,stdout:'',stderr:`taskbelay-${host}: ${output.error}\n`}:{exit_code:0,stdout:output,stderr:''}),example('invalid_data_directory',{argv:[operation,'--json'],environment:{TASKBELAY_DATA_DIR:'relative'}},failure)];
      add(`${host}.lifecycle.${operation}`,`Run ${operation} for the owned ${host} adapter. JSON stdout includes actual local registration status. ZCode action_required requires the returned UI steps; it does not claim the plugin loaded. remove retains Task data; ZCode --confirm-host-removed records actual user confirmation only. A failed command may leave completed registration steps; inspect status before another mutation.`,argvSchema,examples);
    }
  }
  const paths={packageRoot:join(root,'packages/codex'),runtimePath:'/example/runtime/taskbelay',receiptPath:join(home,'codex.json'),configurationDirectory:join(home,'configuration'),configurationPath:join(home,'configuration/config.json')};
  const deps={environment,resolvePaths:async()=>paths,readPackageVersion:async()=> '0.10.5',inspectCoreVersion:async()=>version,inspectRegistrationStatus:async()=>({status:'ready',receipt:true,marketplace:true,plugin:true}),setupRegistration:async()=>({status:'installed',changed:true,fileChanges:[{path:paths.receiptPath,change:'created'}]}),removeRegistration:async()=>({status:'removed',changed:true}),stopPackagedWebUI:async()=>{}};
  for(const operation of ['status','setup','remove']) {
    const args=[operation,'--json'];
    const good=await captured(streams=>codexCLI(args,{...deps,...streams}));
    const bad=await captured(streams=>codexCLI(args,{...deps,...streams,resolvePaths:async()=>{throw new Error('TASKBELAY_DATA_DIR must be absolute')}}));
    add(`codex.lifecycle.${operation}`,`Run ${operation} against the owned Codex adapter. Native registration results and configuration changes are reported separately. remove deregisters; the returned next_step handles package uninstall. After failure inspect status before repeating registration.`,argvSchema,[example('success',{argv:args},good),example('invalid_data_directory',{argv:args,environment:{TASKBELAY_DATA_DIR:'relative'}},bad)]);
  }
}
async function hooks() {
  for(const host of ['codex','claude','zcode']) {
    const event={hook_event_name:'PreToolUse',cwd:parent,tool_name:host==='codex'?'apply_patch':'Write',tool_input:host==='codex'?{command:'*** Begin Patch\n*** Add File: result.txt\n+value\n*** End Patch'}:{file_path:'result.txt',content:'value'}};
    const invoke=async decision=>host==='codex'?captured(({stdout,stderr})=>({code:codexHook({output:stdout,error:stderr,environment:{TASKBELAY_DATA_DIR:parent},readInput:()=>JSON.stringify(event),spawn:()=>({status:0,stdout:JSON.stringify({ok:true,result:decision})})})})):captured(async({stdout})=>{const result=await (host==='claude'?claudeHook:zcodeHook)(event,async()=>decision);if(result)stdout.write(JSON.stringify(result));return {code:0}});
    add(`${host}.hook.pre-tool-use`,'Translate a native PreToolUse event into Core pre-file-write and return the Host permission response. Empty stdout means this guard did not deny; native Host permissions still apply. Shell and unrecognized tools are outside this structured guard. A Core error stops the write with exit 2.',inputObject({hook_event_name:field('Exact native event name.','string',{const:'PreToolUse'}),cwd:field('Absolute actual working directory.'),tool_name:field('Native structured tool name.'),tool_input:field('Original tool arguments; preserve content. apply_patch uses command; Write/Edit use file_path.','object',{additionalProperties:true})}),[
      example('success',event,await invoke({decision:'allow'})),example('denied',event,await invoke({decision:'deny',reason:'Path is outside the current authorized scope.'}))
    ]);
  }
}
async function pets() {
  const state={readiness:'ready',core_identity:version,data_root_digest:'a'.repeat(64),url:'http://127.0.0.1:41731'};
  const dependencies={homeDirectory:parent,environment:{LANG:'en'},platform:'darwin',arch:'arm64',language:'en',resolveCoreRuntime:async()=>({runtimePath:'/example/taskbelay',dataDirectory:parent,packageRoot:parent}),exec:async()=>({stdout:JSON.stringify(state)}),fetchImpl:async()=>({ok:true,json:async()=>state}),platformModule:{bundledPetExecutable:()=>'/example/TaskBelayPet',isBundledPetApplicationAvailable:async()=>true,launchPet:async()=> 'started',shutdownPet:async()=>({code:0})}};
  for(const operation of ['start','stop']) {
    const args=['pet',operation];
    add('taskbelay.pet.'+operation,`Explicitly ${operation} the installed desktop companion on a supported local platform. Plain stdout only; no JSON flag. start verifies Core/service identities; stop ends the companion without starting Core. It does not change any Task.`,argvSchema,[example('success',{argv:args},await captured(streams=>runPet(args,{...dependencies,...streams}))),example('unsupported_platform',{argv:args,platform:'win32-arm64'},await captured(streams=>runPet(args,{...dependencies,...streams,platform:'win32',arch:'arm64'})))]);
  }
}
try {
  await lifecycle();await hostLifecycle();await hooks();await pets();
  await writeFile(join(root,'contracts/management-examples.json'),JSON.stringify(records,null,2)+'\n');
  process.stdout.write(`Exported ${records.length} management/hook contracts from producers with explicitly simulated external drivers.\n`);
} finally {await rm(parent,{recursive:true,force:true})}
