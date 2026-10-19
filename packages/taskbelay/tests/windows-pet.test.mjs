import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Readable } from "node:stream";
import test from "node:test";
import { promptForRequest } from "../lib/cli.mjs";
import { supportsDesktopPet, loadPetPlatform } from "../lib/platform.mjs";
import { ensurePetInstalled } from "../lib/platform/windows/pet-installer.mjs";

test("Windows x64 menu exposes both pet commands and rejects other Windows architectures", async () => {
  assert.equal(supportsDesktopPet("win32", "x64"), true);
  for (const arch of ["ia32", "arm64"])
    assert.equal(supportsDesktopPet("win32", arch), false);
  assert.equal(
    (await loadPetPlatform("win32", "x64")).PET_RUNTIME_DIRECTORY,
    "win32-x64",
  );
  for (const [input, expected] of [
    ["7\n", "start"],
    ["8\n", "stop"],
  ]) {
    let text = "";
    const answers = ["pet", expected];
    const request = await promptForRequest({
      input: Readable.from([input]),
      session: { select: async (_message, entries) => { text += entries.map(entry => entry.label).join("\n"); return answers.shift(); }, close() {} },
      output: {
        write(value) {
          text += value;
        },
      },
      language: "en",
      platform: "win32",
      arch: "x64",
    });
    assert.deepEqual(request, { pet: expected });
    assert.match(text, /Start or show/u);
    assert.match(text, /Stop/u);
  }
});
test("Windows pet installation preserves an existing app, settings and appearances", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-pet-install-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, "package"),
    petDirectory = join(root, "pet");
  const app = join(source, "runtime", "win32-x64", "TaskBelayPet");
  await mkdir(app, { recursive: true });
  await writeFile(join(app, "TaskBelayPet.exe"), "fixture executable");
  const first = await ensurePetInstalled({
    petDirectory,
    sourcePackageRoots: [source],
  });
  assert.equal(first.newlyInstalled, true);
  await writeFile(join(petDirectory, "settings.json"), "retained");
  await writeFile(join(app, "TaskBelayPet.exe"), "new fixture executable");
  const again = await ensurePetInstalled({
    petDirectory,
    sourcePackageRoots: [source],
  });
  assert.equal(again.newlyInstalled, false);
  assert.equal(
    await readFile(first.targetExecutable, "utf8"),
    "fixture executable",
  );
  assert.equal(
    await readFile(join(petDirectory, "settings.json"), "utf8"),
    "retained",
  );
  await ensurePetInstalled({petDirectory,sourcePackageRoots:[source],replaceExisting:true});
  assert.equal(await readFile(first.targetExecutable,"utf8"),"new fixture executable");
  assert.equal(await readFile(join(petDirectory,"settings.json"),"utf8"),"retained");
});

test("pet status requires a native identity or a token-verified live pipe", async t => {
  const { observePetState: macStatus } = await import("../lib/platform/macos/pet.mjs");
  assert.equal(await macStatus({ executable: "/pet", productRoot: "/product", exec: async (_command, args) => {
    assert.deepEqual(args, ["status", "--product-root", "/product"]); return { stdout: "running\n" };
  } }), "running");
  assert.equal(await macStatus({ exec: async () => { throw new Error("unavailable"); } }), "unknown");
  const { observePetState } = await import("../lib/platform/windows/pet.mjs");
  const { EventEmitter } = await import("node:events");
  const root = await mkdtemp(join(tmpdir(), "taskbelay-pet-status-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  assert.equal(await observePetState({ productRoot: root }), "stopped");
  await mkdir(join(root, "pet"));
  const record = join(root, "pet/runtime.json");
  await writeFile(record, JSON.stringify({ pipe: "\\\\.\\pipe\\taskbelay-pet-fixture", token: "secret" }));
  for (const answer of ["running", "wrong"]) {
    const state = await observePetState({ productRoot: root, connectImpl: () => {
      const socket = new EventEmitter(); socket.setTimeout = () => {}; socket.destroy = () => {};
      socket.write = text => { assert.deepEqual(JSON.parse(text), { token: "secret", operation: "status" }); socket.emit("data", JSON.stringify({ result: answer }) + "\n"); };
      queueMicrotask(() => socket.emit("connect")); return socket;
    } });
    assert.equal(state, answer === "running" ? "running" : "unknown");
  }
  assert.match(await readFile(record, "utf8"), /secret/);
});
