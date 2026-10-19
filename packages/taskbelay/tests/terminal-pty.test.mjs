import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import test from "node:test";

const run = promisify(execFile);

test("native macOS PTY accepts arrows, multiple selections, text and cancellation and restores input mode", { skip: process.platform !== "darwin" }, async t => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-terminal-pty-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const fixture = join(root, "terminal.mjs");
  const moduleURL = new URL("../lib/terminal.mjs", import.meta.url).href;
  await writeFile(fixture, `import {createTerminalSession,BACK,CANCEL} from ${JSON.stringify(moduleURL)};
const session=createTerminalSession(process.stdin,process.stdout,{language:"zh-CN",environment:{NO_COLOR:"1"}});
try {
const selected=await session.multiselect("选择升级项",[{value:"codex",label:"Codex"},{value:"disabled",label:"已是最新",disabled:true},{value:"deepseek:web",label:"DeepSeek / web"}]);
process.stdout.write("SELECTION="+JSON.stringify(selected)+"\\n");
const text=await session.question("输入目录");process.stdout.write("TEXT="+text+"\\n");
const previous=await session.select("返回测试",[{value:true,label:"不会执行"}]);process.stdout.write("BACK="+(previous===BACK)+"\\n");
const cancelled=await session.select("退出测试",[{value:true,label:"不会执行"}]);process.stdout.write("CANCEL="+(cancelled===CANCEL)+"\\n");
}finally{session.close();}
`);
  const python = join(root, "pty_check.py");
  await writeFile(python, `import os, pty, subprocess, sys, select, time, termios, fcntl, struct
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 24, 48, 0, 0))
child = subprocess.Popen([sys.argv[1], sys.argv[2]], stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
output = b''
def wait_for(text):
    global output
    wanted = text.encode()
    deadline = time.monotonic() + 10
    while wanted not in output:
        if time.monotonic() > deadline:
            raise AssertionError('missing ' + text + ': ' + output.decode(errors='replace'))
        if select.select([master], [], [], 0.2)[0]:
            chunk = os.read(master, 65536)
            if not chunk: raise AssertionError('terminal closed')
            output += chunk
try:
    wait_for('Esc')
    os.write(master, b' \\x1b[B \\r')
    wait_for('输入目录')
    os.write(master, '/tmp/有 空格'.encode() + b'\\r')
    wait_for('返回测试')
    os.write(master, b'\\x1b[D')
    wait_for('退出测试')
    os.write(master, b'\\x03')
    wait_for('CANCEL=true')
    assert child.wait(timeout=10) == 0
    assert b'SELECTION=["codex","deepseek:web"]' in output
    assert 'TEXT=/tmp/有 空格'.encode() in output
    assert b'BACK=true' in output
    flags=termios.tcgetattr(slave)[3]
    assert flags & termios.ICANON and flags & termios.ECHO
    print('PTY keyboard flow passed; canonical input and echo restored.')
finally:
    if child.poll() is None: child.kill(); child.wait()
    os.close(master); os.close(slave)
`);
  const result = await run("python3", [python, process.execPath, fixture], { timeout: 45000, encoding: "utf8" });
  assert.match(result.stdout, /PTY keyboard flow passed/);
});

test("native PTY traverses the all-current upgrade screen and cancels permanent deletion safely", { skip: process.platform !== "darwin" }, async t => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-menu-pty-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const fixture = join(root, "menu.mjs");
  await writeFile(fixture, `import {runMain} from ${JSON.stringify(new URL("../lib/lifecycle.mjs", import.meta.url).href)};
import {promptForRequest} from ${JSON.stringify(new URL("../lib/menu.mjs", import.meta.url).href)};
import {mkdir,writeFile,readFile} from "node:fs/promises";
const root=process.argv[3];
if(process.argv[2]==="upgrade") {
let checks=0;
const request=await promptForRequest({language:"zh-CN",inspectUpgrade:async options=>{
checks++; process.stdout.write("CHECK="+checks+"\\n");
return [{value:"self",label:"TaskBelay",channel:"npm",status:options.component?"available":"current",disabled:!options.component,hint:options.component?"0.3.8 → "+options.targetVersion:"0.3.8 · 已是最新"}];
}});
process.stdout.write("REQUEST="+JSON.stringify({self:request.self,hosts:request.hosts,version:request.targetVersion})+"\\n");
} else {
const data=root+"/.taskbelay/data";await mkdir(data,{recursive:true});await writeFile(data+"/retained.txt","retained");
const driver=host=>({knownProfiles:async()=>[],observe:async()=>({host,state:"absent",packageInstalled:false,packageVersion:null,hostAvailable:true,issues:[]}),execute:async()=>{throw new Error("must not execute");}});
const result=await runMain(["delete-all","--permanent","--plain"],{homeDirectory:root,environment:{LC_ALL:"zh_CN.UTF-8"},codexDriver:driver("codex"),deepseekDriver:driver("deepseek"),claudeDriver:driver("claude"),zcodeDriver:driver("zcode")});
process.stdout.write("EXIT="+result.code+"\\nDATA="+await readFile(data+"/retained.txt","utf8")+"\\n");
}
`);
  const script = join(root, "flow.py");
  await writeFile(script, `import os,pty,subprocess,sys,select,time,termios,fcntl,struct
for scenario in ['upgrade','cancel']:
    master,slave=pty.openpty()
    fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,80,0,0))
    child=subprocess.Popen([sys.argv[1],sys.argv[2],scenario,sys.argv[3]],stdin=slave,stdout=slave,stderr=slave,start_new_session=True)
    output=b''
    def wait_for(text):
        global output
        deadline=time.monotonic()+15
        while text.encode() not in output:
            if time.monotonic()>deadline: raise AssertionError(text+': '+output.decode(errors='replace'))
            if select.select([master],[],[],0.1)[0]: output+=os.read(master,65536)
    try:
        if scenario=='upgrade':
            wait_for('你想做什么');os.write(master,b'\\x1b[A\\r')
            wait_for('所有组件已是最新版本');os.write(master,b'r')
            wait_for('CHECK=2');os.write(master,b'v')
            wait_for('选择要指定版本的组件');os.write(master,b'\\r')
            wait_for('目标版本');os.write(master,b'\\x150.3.9\\r')
            wait_for('REQUEST=')
            assert b'"self":true,"hosts":[],"version":"0.3.9"' in output
            assert '请至少选择'.encode() not in output
            assert '本地分发包'.encode() not in output
        else:
            wait_for('Ctrl+U');os.write(master,b'\\x03')
            wait_for('DATA=retained')
            assert '已取消，未执行'.encode() in output
            assert b'EXIT=130' in output
            assert b'--confirm-delete' not in output and b'--confirm-permanent' not in output
        assert child.wait(timeout=10)==0
        flags=termios.tcgetattr(slave)[3]
        assert flags&termios.ICANON and flags&termios.ECHO
    finally:
        if child.poll() is None: child.kill();child.wait()
        os.close(master);os.close(slave)
print('Full menu PTY scenarios passed')
`);
  const result = await run("python3", [script, process.execPath, fixture, root], { timeout: 60000, encoding: "utf8" });
  assert.match(result.stdout, /Full menu PTY scenarios passed/);
});
