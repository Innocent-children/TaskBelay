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
