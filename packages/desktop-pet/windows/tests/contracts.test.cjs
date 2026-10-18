const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const vm = require("node:vm");
const { presentation, origin } = require("../observation.cjs");
const {
  AppearanceStore,
  svgSize,
  rows,
  validateCatalog,
} = require("../appearance.cjs");
const { Preferences } = require("../storage.cjs");
const bundled = path.resolve(__dirname, "../../default-appearance");

test("Windows launch accepts the current Core version identity and rejects obsolete or malformed identities", async () => {
  const source = await fs.readFile(path.join(__dirname, "../main.cjs"), "utf8");
  const start = source.indexOf("function parse(argv) {");
  const end = source.indexOf("async function acknowledgeLaunch", start);
  assert.ok(start >= 0 && end > start);
  const context = vm.createContext({ path: path.win32 });
  vm.runInContext(source.slice(start, end), context);
  const version = (await fs.readFile(path.resolve(__dirname, "../../../../CORE_VERSION"), "utf8")).trim();
  const args = identity => [
    "run", "--core-path", "C:\\adapter\\taskbelay.exe", "--data-dir", "C:\\taskbelay\\data",
    "--product-root", "C:\\taskbelay", "--core-identity", identity,
    "--data-root-digest", "a".repeat(64), "--launch-id", "11111111-1111-4111-8111-111111111111",
  ];
  assert.equal(context.parse(args(version)).request.coreIdentity, version);
  for (const identity of [`taskbelay/${version}`, "0.22", "00.22.0", "0..0", "0.22.0-extra"]) {
    assert.throws(() => context.parse(args(identity)), /Incomplete desktop identity/);
  }
});

test("empty bubble layout preserves the character anchor at every supported scale", async () => {
  const source = await fs.readFile(path.join(__dirname, "../main.cjs"), "utf8");
  const start = source.indexOf("  function layout(offset = 0) {");
  const end = source.indexOf("  let deliveredAppearance", start);
  assert.ok(start >= 0 && end > start);
  const context = vm.createContext({
    appearance: { catalog: { canvas: { width: 100, height: 100 }, anchor: { x: 50, y: 90 } } },
    prefs: { value: { scale: 1 } }, anchor: { x: 500, y: 600 },
    tasks: { cards: [{}] }, requestedBubbleHeight: 200, bubbleHeight: 200,
    screen: { getDisplayMatching: () => ({ workArea: { x: 0, y: 0, width: 1600, height: 1200 } }) },
    win: { setBounds() {} },
  });
  vm.runInContext(source.slice(start, end), context);
  for (const scale of [0.5, 0.75, 1, 1.25, 1.5, 2]) {
    context.prefs.value.scale = scale;
    context.tasks.cards = [{}];
    const shown = context.layout();
    const shownHeight = context.bubbleHeight;
    context.tasks.cards = [];
    const hidden = context.layout();
    assert.equal(context.bubbleHeight, 0);
    assert.equal(hidden.height, Math.ceil(144 * scale + 16));
    assert.equal(hidden.x, shown.x);
    assert.equal(hidden.y, shown.y + shownHeight, "removing the bubble preserves the character screen position");
    context.tasks.cards = [{}];
    assert.deepEqual(context.layout(), shown);
  }
});

test("the bubble starts hidden and author styles respect hidden controls", async () => {
  const html = await fs.readFile(path.join(__dirname, "../view.html"), "utf8");
  const css = await fs.readFile(path.join(__dirname, "../view.css"), "utf8");
  assert.match(html, /id="bubble" hidden/);
  assert.match(css, /\[hidden\]\s*\{\s*display:\s*none\s*!important;/);
});

test("presentation follows Core lifecycle and never replays completion after a discontinuity", () => {
  const task = {
    task_id: "task-a",
    lifecycle: "active",
    status: "active",
    archived: false,
  };
  const active = presentation(task, null);
  const done = presentation({ ...task, lifecycle: "done" }, active);
  assert.equal(done.clip, "complete");
  assert.equal(done.prompt, true);
  assert.equal(
    presentation({ ...task, lifecycle: "done" }, done).prompt,
    false,
  );
  assert.equal(
    presentation({ ...task, lifecycle: "done" }, active, true, true, false)
      .prompt,
    false,
  );
  const disconnected = presentation(null, active, false);
  assert.equal(disconnected.clip, "disconnected");
  assert.equal(disconnected.summary, task);
  assert.equal(disconnected.stale, true);
  assert.equal(
    presentation({ ...task, lifecycle: "done" }, disconnected).prompt,
    false,
  );
  assert.equal(
    presentation({ ...task, status: "active" }, active)
      .clip,
    "working",
  );
  assert.equal(
    presentation({ ...task, lifecycle: "blocked", blocker: "review" }, active)
      .clip,
    "blocked",
  );
  assert.equal(presentation(null, active, true, true).phase, "missing");
  assert.equal(presentation({ ...task, archived: true }, active).rest, true);
  assert.equal(origin("http://127.0.0.1:45678"), true);
  assert.equal(origin("http://localhost:45678"), false);
  assert.equal(origin("https://example.com"), false);
});
test("default native artwork retains nine clips and 57 PNG frames", async () => {
  const store = new AppearanceStore("", bundled);
  const value = await store.load(bundled);
  assert.equal(Object.keys(value.catalog.clips).length, 9);
  assert.equal(
    Object.values(value.catalog.clips).reduce((n, c) => n + c.frames.length, 0),
    57,
  );
  assert.equal(
    rows.reduce((n, [, d]) => n + d.length, 0),
    57,
  );
  assert.throws(
    () =>
      validateCatalog({
        ...value.catalog,
        clips: {
          ...value.catalog.clips,
          complete: { ...value.catalog.clips.complete, loop_range: [0, 1] },
        },
      }),
    /loop/,
  );
});
test("SVG external content, unsafe paths and failed reimport preserve installed artwork", async (t) => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "taskbelay-pet-import-"));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  const source = path.join(root, "中文 source");
  await fs.mkdir(source);
  const svg =
    '<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128"><rect width="128" height="128" fill="#ff9900"/></svg>';
  await fs.writeFile(
    path.join(source, "pet.json"),
    JSON.stringify({ id: "orange", name: "橙色", image: "pet.svg" }),
  );
  await fs.writeFile(path.join(source, "pet.svg"), svg);
  const store = new AppearanceStore(path.join(root, "installed"), bundled);
  const result = await store.import(source);
  assert.equal(result.id, "orange");
  assert.equal(result.catalog.clips.complete.loop_range, null);
  await fs.writeFile(
    path.join(source, "pet.svg"),
    svg.replace("<rect", "<script"),
  );
  await assert.rejects(store.import(source));
  assert.equal(
    (await store.selected("orange")).frames["static.svg"],
    result.frames["static.svg"],
  );
  await fs.writeFile(
    path.join(source, "pet.json"),
    JSON.stringify({ id: "orange", name: "Orange", image: "../outside.png" }),
  );
  await assert.rejects(store.import(source), /relative/);
  assert.throws(() =>
    svgSize(Buffer.from('<!DOCTYPE svg SYSTEM "https://example.com/a"><svg/>')),
  );
  assert.throws(() =>
    svgSize(Buffer.from(svg.replace("<rect", '<use href="file:///C:/x"'))),
  );
});
test("native appearance imports enforce timing and total frame limits before replacement", async (t) => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "taskbelay-pet-limits-"));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  const source = path.join(root, "source");
  await fs.mkdir(path.join(source, "Assets"), { recursive: true });
  await fs.writeFile(
    path.join(source, "pet.json"),
    JSON.stringify({ id: "limits", name: "Animation limits" }),
  );
  await fs.writeFile(
    path.join(source, "Assets/frame.svg"),
    '<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><rect width="1" height="1" fill="#ff9900"/></svg>',
  );
  const catalog = {
    canvas: { width: 1, height: 1 },
    anchor: { x: 0, y: 1 },
    clips: Object.fromEntries(
      ["idle", "working", "blocked", "complete", "disconnected"].map((clip) => [
        clip,
        {
          frames: Array(clip === "idle" ? 508 : 1).fill("frame.svg"),
          fps: clip === "idle" ? 0.1 : 120,
          loop_range: clip === "complete" ? null : [0, 0],
          rest_frame: 0,
        },
      ]),
    ),
  };
  catalog.clips.working.frame_durations_ms = [9];
  catalog.clips.complete.frame_durations_ms = [60000];
  const manifest = path.join(source, "animations.json");
  await fs.writeFile(manifest, JSON.stringify(catalog));
  const store = new AppearanceStore(path.join(root, "installed"), bundled);
  const installed = await store.import(source);
  assert.deepEqual(installed.catalog, catalog);

  for (const [label, mutate, error] of [
    ["fps below 0.1", (value) => { value.clips.idle.fps = 0.09; }, /animation clip/],
    ["fps above 120", (value) => { value.clips.idle.fps = 120.1; }, /animation clip/],
    ["duration below 9 ms", (value) => { value.clips.working.frame_durations_ms = [8]; }, /frame durations/],
    ["duration above 60000 ms", (value) => { value.clips.complete.frame_durations_ms = [60001]; }, /frame durations/],
    ["513 total frame references", (value) => { value.clips.idle.frames.push("frame.svg"); }, /512 frame references/],
  ]) {
    const invalid = structuredClone(catalog);
    mutate(invalid);
    await fs.writeFile(manifest, JSON.stringify(invalid));
    await assert.rejects(store.import(source), error, label);
    assert.deepEqual(await store.selected("limits"), installed, label);
    assert.deepEqual(await fs.readdir(path.join(root, "installed")), ["limits"]);
  }
});
test("concurrent preference updates keep independent selections and exact scale", async (t) => {
  const root = await fs.mkdtemp(
    path.join(os.tmpdir(), "taskbelay-pet-settings-"),
  );
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  const prefs = new Preferences(root);
  await prefs.load();
  await Promise.all([
    prefs.update((p) => {
      p.scale = 1.5;
    }),
    prefs.update((p) => {
      p.pinned_tasks.a = "task-a";
    }),
    prefs.update((p) => {
      p.selected_appearance = "orange";
    }),
  ]);
  const reopened = new Preferences(root);
  await reopened.load();
  assert.equal(reopened.value.scale, 1.5);
  assert.equal(reopened.value.pinned_tasks.a, "task-a");
  assert.equal(reopened.value.selected_appearance, "orange");
});
