const {
  app,
  BrowserWindow,
  Tray,
  Menu,
  dialog,
  ipcMain,
  screen,
  shell,
  nativeImage,
  powerMonitor,
} = require("electron");
const fs = require("node:fs/promises");
const path = require("node:path");
const net = require("node:net");
const { createHash, randomUUID } = require("node:crypto");
const { Preferences, writeJSON, safePath } = require("./storage.cjs");
const { AppearanceStore } = require("./appearance.cjs");
const { Observer, presentation } = require("./observation.cjs");
const { TaskCollection } = require("./task-collection.cjs");
const texts = {
  zh: {
    title: "TaskBelay 桌面宠物",
    selectTask: "选择任务",
    noSelection: "恢复自动",
    pin: "固定关注", tasks: "个未完成", dismiss: "关闭提示",
    appearance: "选择形象",
    import: "导入形象…",
    default: "默认形象",
    size: "宠物大小",
    animate: "启用动画",
    idleActivities: "启用待机活动",
    hide: "隐藏",
    show: "显示",
    quit: "退出",
    open: "打开 WebUI",
    idle: "未选择任务",
    missing: "任务不可用",
    active: "任务进行中",
    blocked: "任务受阻",
    done: "任务已完成",
    cancelled: "任务已取消",
    archived: "任务已归档",
    disconnected: "连接已断开",
    stale: "上次保存状态",
    error: "操作未完成",
    updated: "任务更新",
    lastSync: "上次同步",
    readOnly: "只读",
  },
  en: {
    title: "TaskBelay Desktop Pet",
    selectTask: "Select task",
    noSelection: "Follow automatically",
    pin: "Pin task", tasks: "unfinished", dismiss: "Dismiss",
    appearance: "Choose appearance",
    import: "Import appearance…",
    default: "Default appearance",
    size: "Pet size",
    animate: "Enable animations",
    idleActivities: "Enable idle activities",
    hide: "Hide",
    show: "Show",
    quit: "Quit",
    open: "Open WebUI",
    idle: "No selected task",
    missing: "Task unavailable",
    active: "Task in progress",
    blocked: "Task blocked",
    done: "Task completed",
    cancelled: "Task cancelled",
    archived: "Task archived",
    disconnected: "Disconnected",
    stale: "Last saved state",
    error: "Operation failed",
    updated: "Task updated",
    lastSync: "Last sync",
    readOnly: "Read only",
  },
};
function parse(argv) {
  const operation = argv[0];
  if (!["run", "stop"].includes(operation) || (argv.length - 1) % 2)
    throw new Error("Invalid desktop entry arguments");
  const values = {};
  for (let i = 1; i < argv.length; i += 2) {
    const key = argv[i];
    if (
      Object.hasOwn(values, key) ||
      ![
        "--core-path",
        "--data-dir",
        "--product-root",
        "--core-identity",
        "--data-root-digest",
        "--launch-id",
      ].includes(key) ||
      !argv[i + 1] ||
      argv[i + 1].includes("\0")
    )
      throw new Error("Invalid desktop entry argument");
    values[key] = argv[i + 1];
  }
  const request = {
    corePath: values["--core-path"],
    dataDirectory: values["--data-dir"],
    productRoot: values["--product-root"],
    coreIdentity: values["--core-identity"],
    dataRootDigest: values["--data-root-digest"],
    launchID: values["--launch-id"],
  };
  if (!path.isAbsolute(request.productRoot ?? ""))
    throw new Error("Product root must be absolute");
  if (
    operation === "run" &&
    (!path.isAbsolute(request.corePath ?? "") ||
      !path.isAbsolute(request.dataDirectory ?? "") ||
      !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(request.coreIdentity ?? "") ||
      !/^[a-f0-9]{64}$/.test(request.dataRootDigest ?? ""))
  )
    throw new Error("Incomplete desktop identity");
  if (
    operation === "stop" &&
    (request.dataDirectory ||
      request.coreIdentity ||
      request.dataRootDigest ||
      (request.corePath && !path.isAbsolute(request.corePath)))
  )
    throw new Error("Invalid stop filter");
  if (operation === "run" ? !/^[a-f0-9-]{36}$/.test(request.launchID ?? "") : request.launchID !== undefined) throw new Error("Invalid launch identity");
  return { operation, request };
}
async function acknowledgeLaunch(request, result) {
  if (!request.launchID) return;
  await writeJSON(path.join(request.productRoot, "pet", `.launch-${request.launchID}.json`), result);
}
async function createDesktop(request) {
  Menu.setApplicationMenu(null);
  const root = path.join(request.productRoot, "pet");
  await fs.mkdir(root, { recursive: true });
  const prefs = new Preferences(root);
  await prefs.load();
  const labels =
    texts[app.getLocale().toLowerCase().startsWith("zh") ? "zh" : "en"];
  const decoder = new BrowserWindow({
    show: false,
    webPreferences: {
      sandbox: true,
      contextIsolation: true,
      nodeIntegration: false,
    },
  });
  await decoder.loadFile(path.join(__dirname, "decode.html"));
  async function decodeAtlas(bytes, size, cell, rows) {
    const mime = bytes.subarray(0, 4).toString() === "RIFF" ? "webp" : "png";
    return Object.fromEntries(
      Object.entries(
        await decoder.webContents.executeJavaScript(`(async()=>{
      const img=new Image();img.src=${JSON.stringify(`data:image/${mime};base64,${bytes.toString("base64")}`)};await img.decode();
      if(img.naturalWidth!==${size.width}||img.naturalHeight!==${size.height})throw new Error('Atlas size changed');
      const canvas=document.createElement('canvas');canvas.width=${cell.width};canvas.height=${cell.height};const ctx=canvas.getContext('2d');const output={};
      const rows=${JSON.stringify(rows)};for(let row=0;row<rows.length;row++)for(let col=0;col<rows[row][1].length;col++){
        ctx.clearRect(0,0,canvas.width,canvas.height);ctx.drawImage(img,col*canvas.width,row*canvas.height,canvas.width,canvas.height,0,0,canvas.width,canvas.height);
        output[rows[row][0]+'/'+col+'.png']=canvas.toDataURL('image/png').split(',')[1];
      }return output;
    })()`),
      ).map(([key, value]) => [key, Buffer.from(value, "base64")]),
    );
  }
  const bundled = path.join(__dirname, "default-appearance");
  const store = new AppearanceStore(
    path.join(root, "appearances"),
    bundled,
    decodeAtlas,
    async (bytes, canvas) => {
      await decoder.webContents.executeJavaScript(`(async()=>{
        const image = new Image(); image.src = ${JSON.stringify("data:image/png;base64," + bytes.toString("base64"))};
        await image.decode();
        if (image.naturalWidth !== ${canvas.width} || image.naturalHeight !== ${canvas.height}) throw new Error('Invalid raster dimensions');
      })()`);
    },
  );
  let warning = prefs.warning ?? "",
    appearance;
  try {
    appearance = await store.selected(prefs.value.selected_appearance);
  } catch (error) {
    warning = error.message;
    appearance = await store.selected(null);
  }
  let serial = 1;
  appearance.serial = serial;
  const win = new BrowserWindow({
    show: false,
    width: 360,
    height: 440,
    transparent: true,
    frame: false,
    alwaysOnTop: true,
    skipTaskbar: true,
    resizable: false,
    hasShadow: false,
    webPreferences: {
      preload: path.join(__dirname, "preload.cjs"),
      sandbox: true,
      contextIsolation: true,
      nodeIntegration: false,
      backgroundThrottling: true,
    },
  });
  win.setAlwaysOnTop(true, "floating");
  win.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  win.webContents.on("will-navigate", (event) => event.preventDefault());
  win.webContents.session.setPermissionRequestHandler(
    (_contents, _permission, callback) => callback(false),
  );
  const observer = new Observer(request);
  let selected = prefs.value.pinned_tasks[request.dataRootDigest] ?? null,
    display = presentation(null, null, true, Boolean(selected)),
    picker = null,
    visible = true,
    sleeping = false,
    continuous = false,
    timer,
    generation = 0,
    pickerGeneration = 0,
    lastSyncAt = null,
    walking,
    drag,
    closing = false,
    rendering = false;
  const tasks = new TaskCollection(() => Date.now());
  tasks.pin(selected);
  let bubbleHeight = 0;
  let requestedBubbleHeight = 0;
  const work = screen.getPrimaryDisplay().workArea;
  let anchor = prefs.value.position ?? {
    x: work.x + work.width - 200,
    y: work.y + work.height - 50,
  };
  function stopWalk() {
    clearInterval(walking);
    walking = null;
    layout();
  }
  function layout(offset = 0) {
    const { canvas, anchor: artAnchor } = appearance.catalog;
    const scale = prefs.value.scale;
    const available = screen.getDisplayMatching({ x: Math.round(anchor.x), y: Math.round(anchor.y), width: 1, height: 1 }).workArea;
    bubbleHeight = tasks.cards.length === 0 ? 0 : Math.max(70, Math.min(requestedBubbleHeight, available.height - 144 * scale - 24));
    const factor = (144 * scale) / Math.max(canvas.width, canvas.height);
    const width = Math.ceil(Math.max(304, 144 * scale + 24)),
      height = Math.ceil(bubbleHeight + 144 * scale + 16);
    const bounds = {
      x: Math.round(
        anchor.x -
          width / 2 +
          (canvas.width / 2 - artAnchor.x) * factor +
          offset,
      ),
      y: Math.round(
        anchor.y -
          bubbleHeight -
          (144 * scale - canvas.height * factor) / 2 -
          artAnchor.y * factor,
      ),
      width,
      height,
    };
    const area = screen.getDisplayMatching(bounds).workArea;
    bounds.x = Math.max(
      area.x,
      Math.min(bounds.x, area.x + area.width - width),
    );
    bounds.y = Math.max(
      area.y,
      Math.min(bounds.y, area.y + area.height - height),
    );
    win.setBounds(bounds);
    return bounds;
  }
  let deliveredAppearance = null;
  function publish() {
    if (!win.isDestroyed() && rendering) {
      win.webContents.send("pet:state", {
        preferences: prefs.value,
        appearance:
          deliveredAppearance === appearance.serial ? null : appearance,
        labels,
        display,
        cards: tasks.cards,
        pinnedTaskID: tasks.pinned,
        bubbleHeight,
        visible: visible && !sleeping,
        warning,
        picker,
        lastSyncAt,
      });
      deliveredAppearance = appearance.serial;
      display = { ...display, prompt: false };
      tasks.consumePrompts();
    }
  }
  async function failure(error) {
    warning = error.message;
    publish();
  }
  const guarded = (fn) => () => Promise.resolve().then(fn).catch(failure);
  async function switchAppearance(id, loaded) {
    const next = loaded ?? (await store.selected(id));
    await prefs.update((p) => {
      p.selected_appearance = id;
    });
    appearance = { ...next, serial: ++serial };
    continuous = false;
    display = { ...display, prompt: false };
    stopWalk();
    publish();
    await menu();
  }
  async function importAppearance() {
    const result = await dialog.showOpenDialog(win, {
      properties: ["openDirectory"],
      title: labels.import,
    });
    if (result.canceled) return;
    const loaded = await store.import(result.filePaths[0]);
    await switchAppearance(loaded.id, loaded);
  }
  async function scale(value) {
    const old = anchor;
    await prefs.update((p) => {
      p.scale = value;
      p.position = old;
    });
    stopWalk();
    publish();
    await menu();
  }
  async function visibility(value) {
    visible = value;
    continuous = false;
    observer.cancel();
    clearTimeout(timer);
    generation++;
    pickerGeneration++;
    picker = null;
    stopWalk();
    if (value) {
      win.showInactive();
      await poll();
    } else win.hide();
    publish();
    await menu();
  }
  async function menu() {
    const appearances = await store.list();
    const items = [
      { label: labels.open, click: guarded(open) },
      { label: labels.pin + "…", click: guarded(() => showPicker(1)) },
      { label: labels.noSelection, click: guarded(() => select(null)) },
      { type: "separator" },
      {
        label: labels.appearance,
        submenu: [
          {
            label: labels.default,
            type: "radio",
            checked: !prefs.value.selected_appearance,
            click: guarded(() => switchAppearance(null)),
          },
          ...appearances.map((a) => ({
            label: a.name,
            type: "radio",
            checked: prefs.value.selected_appearance === a.id,
            click: guarded(() => switchAppearance(a.id)),
          })),
          { type: "separator" },
          { label: labels.import, click: guarded(importAppearance) },
        ],
      },
      {
        label: labels.size,
        submenu: [0.5, 0.75, 1, 1.25, 1.5, 2].map((value) => ({
          label: `${value * 100}%`,
          type: "radio",
          checked: prefs.value.scale === value,
          click: guarded(() => scale(value)),
        })),
      },
      {
        label: labels.animate,
        type: "checkbox",
        checked: prefs.value.animations_enabled,
        click: guarded(async () => {
          await prefs.update((p) => {
            p.animations_enabled = !p.animations_enabled;
          });
          publish();
          await menu();
        }),
      },
      {
        label: labels.idleActivities,
        type: "checkbox",
        checked: prefs.value.idle_activities_enabled,
        click: guarded(async () => {
          await prefs.update((p) => {
            p.idle_activities_enabled = !p.idle_activities_enabled;
          });
          stopWalk();
          publish();
          await menu();
        }),
      },
      { type: "separator" },
      {
        label: visible ? labels.hide : labels.show,
        click: guarded(() => visibility(!visible)),
      },
      { label: labels.quit, click: () => app.quit() },
    ];
    const result = Menu.buildFromTemplate(items);
    tray.setContextMenu(result);
    return result;
  }
  async function open(id = selected) {
    const card = tasks.cards.find(c => c.taskID === id);
    if (id !== null && !card && id !== selected && !picker?.items.some(t => t.task_id === id)) throw new Error("Choose a displayed task");
    const missing = id === selected && display.phase === "missing";
    await observer.connect();
    await shell.openExternal(observer.url + "/tasks" + (id && !missing ? "/" + encodeURIComponent(id) : ""));
    tasks.acknowledge(id);
    selected = tasks.focus;
    display = tasks.display;
    publish();
  }
  async function showPicker(page) {
    const session = ++pickerGeneration;
    if (!page) {
      picker = null;
      publish();
      return;
    }
    await observer.connect();
    const result = await observer.list(Math.max(1, page));
    if (session !== pickerGeneration || !visible) return;
    picker = result;
    stopWalk();
    publish();
  }
  async function select(id) {
    if (
      id !== null &&
      (typeof id !== "string" || !picker?.items.some((t) => t.task_id === id) && !tasks.cards.some(c => c.taskID === id))
    )
      throw new Error("Choose a displayed task");
    await prefs.update((p) => {
      if (id) p.pinned_tasks[request.dataRootDigest] = id;
      else delete p.pinned_tasks[request.dataRootDigest];
    });
    tasks.pin(id);
    selected = tasks.focus;
    picker = null;
    continuous = false;
    await poll();
  }
  async function poll() {
    clearTimeout(timer);
    observer.cancel();
    const round = ++generation;
    if (!visible || sleeping || closing) return;
    observer.controller = new AbortController();
    try {
      await observer.connect();
      const summaries = new Map();
      let readiness = "ready";
      for (const lifecycle of ["blocked", "active"]) {
        for (let page = 1; ; page++) {
          const list = await observer.list(page, lifecycle);
          if (round !== generation) return;
          readiness = list.readiness ?? readiness;
          for (const item of list.items) summaries.set(item.task_id, item);
          if (!list.has_next) break;
        }
      }
      const ids = new Set([...tasks.observedIDs].filter(id => !summaries.has(id)));
      if (tasks.pinned) ids.add(tasks.pinned);
      for (const id of ids) {
        const detail = await observer.detail(id);
        if (round !== generation) return;
        if (detail) { summaries.set(id, detail.summary); readiness = detail.readiness ?? readiness; }
        else summaries.delete(id);
      }
      if (!continuous) tasks.interrupt();
      tasks.update([...summaries.values()], readiness);
      selected = tasks.focus;
      display = tasks.display;
      lastSyncAt = new Date().toISOString();
      continuous = true;
    } catch (error) {
      if (round !== generation) return;
      tasks.disconnect();
      display = presentation(null, display, false, Boolean(selected));
      continuous = false;
    }
    if (round === generation) {
      publish();
      timer = setTimeout(poll, tasks.remainingHold ? Math.min(3000, tasks.remainingHold) : 3000);
    }
  }
  async function command(name, value) {
    if (name === "hit" && typeof value === "boolean") {
      win.setIgnoreMouseEvents(!value, { forward: true });
      return;
    }
    if (name === "menu") return (await menu()).popup({ window: win });
    if (name === "open") return open(value === undefined ? selected : value);
    if (name === "dismiss" && tasks.cards.some(c => c.taskID === value)) {
      tasks.acknowledge(value); selected = tasks.focus; display = tasks.display; publish(); return;
    }
    if (name === "bubble-height" && Number.isFinite(value)) {
      requestedBubbleHeight = Math.max(0, Math.min(value, 1000));
      layout(); publish(); return;
    }
    if (name === "picker") return showPicker(Number(value));
    if (name === "select") return select(value);
    if (name === "drag") {
      if (value === "start") {
        stopWalk();
        drag = { cursor: screen.getCursorScreenPoint(), anchor: { ...anchor } };
      }
      if (drag && (value === "move" || value === "end")) {
        const point = screen.getCursorScreenPoint();
        anchor = {
          x: drag.anchor.x + point.x - drag.cursor.x,
          y: drag.anchor.y + point.y - drag.cursor.y,
        };
        layout();
        if (value === "end") {
          const previous = drag.anchor;
          drag = null;
          try {
            await prefs.update((p) => {
              p.position = anchor;
            });
          } catch (error) {
            anchor = previous;
            layout();
            throw error;
          }
        }
      }
      return;
    }
    if (name === "walk") {
      stopWalk();
      if (
        ![-1, 0, 1].includes(value) ||
        !value ||
        !visible ||
        sleeping ||
        drag ||
        picker ||
        display.stale ||
        !["idle", "done", "cancelled", "archived"].includes(display.phase) ||
        !prefs.value.idle_activities_enabled ||
        !prefs.value.animations_enabled
      )
        return;
      const start = Date.now(),
        distance = (40 + Math.random() * 40) * value;
      walking = setInterval(() => {
        const moved =
          Math.min(
            Math.abs(distance),
            ((Date.now() - start) / 1000) * 20 * prefs.value.scale,
          ) * value;
        layout(moved);
        if (Math.abs(moved) >= Math.abs(distance)) {
          clearInterval(walking);
          walking = null;
          win.webContents.send("pet:walk-complete");
        }
      }, 33);
      return;
    }
    throw new Error("Unsupported desktop interaction");
  }
  ipcMain.handle("pet:command", async (event, name, value) => {
    if (event.sender !== win.webContents) throw new Error("Unknown window");
    try {
      return await command(name, value);
    } catch (error) {
      await failure(error);
      throw error;
    }
  });
  ipcMain.on("pet:ready", (event) => {
    if (event.sender === win.webContents) {
      deliveredAppearance = null;
      rendering = true;
      publish();
    }
  });
  const tray = new Tray(nativeImage.createFromPath(path.join(__dirname, "taskbelay-mark-32.png")));
  tray.setToolTip(labels.title);
  tray.on(
    "double-click",
    guarded(() => visibility(true)),
  );
  await win.loadFile(path.join(__dirname, "view.html"));
  layout();
  await menu();
  win.showInactive();
  await poll();
  powerMonitor.on("suspend", () => {
    sleeping = true;
    continuous = false;
    observer.cancel();
    generation++;
    clearTimeout(timer);
    stopWalk();
    publish();
  });
  powerMonitor.on("resume", () => {
    sleeping = false;
    continuous = false;
    poll();
  });
  screen.on("display-metrics-changed", () => layout());
  screen.on("display-removed", () => layout());
  win.on("close", (event) => {
    if (!closing) {
      event.preventDefault();
      visibility(false).catch(failure);
    }
  });
  app.on("before-quit", () => {
    closing = true;
    observer.cancel();
    clearTimeout(timer);
    clearInterval(walking);
    tray.destroy();
    decoder.destroy();
  });
  return {
    win,
    decoder,
    prefs,
    store,
    request,
    visibility,
    scale,
    switchAppearance,
    showPicker,
    poll,
    command,
    get display() {
      return display;
    },
  };
}
async function sendExisting(root, operation, request) {
  const record = JSON.parse(
    await fs.readFile(path.join(root, "runtime.json"), "utf8"),
  );
  return new Promise((resolve, reject) => {
    let data = "";
    const socket = net.connect(record.pipe);
    socket.setTimeout(8000);
    socket.on("connect", () =>
      socket.write(
        JSON.stringify({
          token: record.token,
          operation,
          corePath: request.corePath ?? null,
          dataRootDigest: request.dataRootDigest ?? null,
          coreIdentity: request.coreIdentity ?? null,
        }) + "\n",
      ),
    );
    socket.on("data", (chunk) => {
      data += chunk;
      if (data.includes("\n")) {
        socket.end();
        const value = JSON.parse(data);
        value.error ? reject(new Error(value.error)) : resolve(value.result);
      }
    });
    socket.on("error", reject);
    socket.on("timeout", () => {
      socket.destroy();
      reject(new Error("Desktop instance did not acknowledge request"));
    });
  });
}
async function boot() {
  // The launcher intentionally closes its confirmation pipes after startup.
  // A vanished launcher is not an uncaught GUI exception.
  for (const stream of [process.stdout, process.stderr]) {
    stream.on("error", (error) => {
      if (error.code !== "EPIPE") app.exit(1);
    });
  }
  if (process.platform !== "win32" || process.arch !== "x64")
    throw new Error("Windows x64 desktop required");
  const { operation, request } = parse(
    process.argv.slice(app.isPackaged ? 1 : 2),
  );
  const root = path.join(request.productRoot, "pet");
  await fs.mkdir(root, { recursive: true });
  await safePath(request.productRoot, "pet/runtime.json");
  app.setPath("userData", path.join(root, "windows-runtime"));
  app.setName("TaskBelay Desktop Pet");
  if (!app.requestSingleInstanceLock()) {
    const result = await sendExisting(root, operation, request);
    await acknowledgeLaunch(request, {result});
    app.exit(0);
    return;
  }
  if (operation === "stop") {
    app.exit(0);
    return;
  }
  await app.whenReady();
  const desktop = await createDesktop(request);
  const token = randomUUID(),
    pipe =
      "\\\\.\\pipe\\taskbelay-pet-" +
      createHash("sha256")
        .update(root.toLowerCase())
        .digest("hex")
        .slice(0, 32);
  const server = net.createServer((socket) => {
    let data = "";
    socket.setTimeout(3000, () => socket.destroy());
    socket.on("error", () => {});
    socket.on("data", (chunk) => {
      data += chunk;
      if (data.length > 8192) {
        socket.destroy();
        return;
      }
      if (!data.includes("\n")) return;
      Promise.resolve()
        .then(async () => {
          const message = JSON.parse(data);
          if (message.token !== token)
            throw new Error("Desktop identity changed");
          if (message.operation === "status") {
            socket.end(JSON.stringify({ result: "running" }) + "\n");
          } else if (message.operation === "stop") {
            if (
              message.corePath &&
              path.resolve(message.corePath).toLowerCase() !==
                path.resolve(request.corePath).toLowerCase()
            ) {
              socket.end(JSON.stringify({ result: "unmatched" }) + "\n");
              return;
            }
            socket.end(JSON.stringify({ result: "stopped" }) + "\n");
            setImmediate(() => app.quit());
          } else if (message.operation === "run") {
            if (
              path.resolve(message.corePath ?? "").toLowerCase() !==
                path.resolve(request.corePath).toLowerCase() ||
              message.dataRootDigest !== request.dataRootDigest ||
              message.coreIdentity !== request.coreIdentity
            ) {
              throw new Error(
                "Another desktop instance holds a different Core or data directory",
              );
            }
            await desktop.visibility(true);
            socket.end(JSON.stringify({ result: "restored" }) + "\n");
          } else throw new Error("Unknown desktop request");
        })
        .catch((error) =>
          socket.end(JSON.stringify({ error: error.message }) + "\n"),
        );
    });
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(pipe, resolve);
  });
  await writeJSON(path.join(root, "runtime.json"), {
    pid: process.pid,
    executable_path: process.execPath,
    core_path: request.corePath,
    pipe,
    token,
  });
  app.on("will-quit", () => {
    server.close();
    require("node:fs").rmSync(path.join(root, "runtime.json"), { force: true });
  });
  await acknowledgeLaunch(request, {result:"ready"});
}
if (require.main === module)
  boot().catch(async (error) => {
    try {
      const {request} = parse(process.argv.slice(app.isPackaged ? 1 : 2));
      await acknowledgeLaunch(request, {error:error.message});
    } catch {}
    process.stderr.write(error.message + "\n");
    app.exit(1);
  });
module.exports = { createDesktop, parse };
