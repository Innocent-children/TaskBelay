import { emitKeypressEvents } from "node:readline";

export const BACK = Symbol("back");
export const CANCEL = Symbol("cancel");

/**
 * One input owner switches raw mode only while a prompt is visible. Keys typed
 * during maintenance are discarded, so they cannot approve a later operation.
 */
export function createTerminalSession(input, output, { language = "en", environment = process.env } = {}) {
  const zh = language === "zh-CN";
  const cursor = environment.TERM !== "dumb";
  let pending = null;
  let closed = false;
  let previousRaw = false;
  emitKeypressEvents(input);
  const finish = value => {
    const active = pending;
    if (!active) return;
    pending = null;
    input.setRawMode?.(previousRaw);
    input.pause?.();
    output.write(cursor ? "\u001b[?25h" : "");
    output.write("\n");
    active.resolve(value);
  };
  const onKey = (text, key = {}) => {
    if (!pending) return;
    if (key.ctrl && ["c", "d"].includes(key.name)) { closed = true; finish(CANCEL); return; }
    pending.key(text, key);
  };
  const onEnd = () => { closed = true; finish(CANCEL); };
  input.on("keypress", onKey);
  input.on("end", onEnd);
  input.on("error", onEnd);
  function prompt(start) {
    if (closed || input.readableEnded) return Promise.resolve(CANCEL);
    if (pending) throw new Error("a terminal prompt is already active");
    return new Promise(resolve => {
      previousRaw = Boolean(input.isRaw);
      pending = { resolve, key: () => {} };
      try {
        input.setRawMode?.(true);
        start();
        input.resume?.();
      } catch (error) { finish(CANCEL); throw error; }
    });
  }
  function choose(message, entries, { multiple = false, initial = [], initialValue, back = true, shortcuts = {}, onChange = () => {} } = {}) {
    return prompt(() => {
      let index = Math.max(0, entries.findIndex(entry => entry.value === initialValue && !entry.disabled));
      if (entries[index]?.disabled) index = entries.findIndex(entry => !entry.disabled);
      const selected = new Set(initial.filter(value => entries.some(entry => entry.value === value && !entry.disabled)));
      let previousLines = 0;
      let error = "";
      const draw = () => {
        if (previousLines && cursor) output.write(`\u001b[${previousLines}A\r\u001b[0J`);
        const columns = Math.max(20, output.columns ?? 80);
        const capacity = Math.max(3, (output.rows ?? 24) - 6);
        const start = Math.max(0, Math.min(index - Math.floor(capacity / 2), entries.length - capacity));
        const lines = [message];
        if (start > 0) lines.push("  ↑ …");
        for (let i = start; i < Math.min(entries.length, start + capacity); i++) {
          const entry = entries[i];
          const mark = multiple ? selected.has(entry.value) ? "[x]" : "[ ]" : i === index ? "●" : "○";
          lines.push(`${i === index ? "›" : " "} ${mark} ${entry.label}${entry.hint ? ` — ${entry.hint}` : ""}${entry.disabled ? zh ? "（不可选）" : " (unavailable)" : ""}`);
        }
        if (start + capacity < entries.length) lines.push("  ↓ …");
        if (error) lines.push(error);
        lines.push(multiple ? zh ? "↑↓ 移动 · 空格勾选 · Enter 继续 · A 全选 · Esc 返回" : "↑↓ move · Space select · Enter continue · A all · Esc back"
          : zh ? `↑↓ 移动 · Enter 确认 · ${back ? "Esc 返回 · " : ""}Ctrl+C 退出` : `↑↓ move · Enter confirm · ${back ? "Esc back · " : ""}Ctrl+C exit`);
        if (Object.keys(shortcuts).length) lines.push(Object.entries(shortcuts).map(([key, value]) => `${key.toUpperCase()} ${value}`).join(" · "));
        output.write((cursor ? "\u001b[?25l" : "") + lines.map(line => fitLine(line, columns - 1)).join("\n") + "\n");
        previousLines = lines.length;
      };
      pending.key = (_text, key) => {
        if (Object.hasOwn(shortcuts, key.name) && !key.ctrl) { finish({ action: key.name, selected: [...selected] }); return; }
        if ((key.name === "escape" || key.name === "left") && back) { finish(BACK); return; }
        if (["up", "down"].includes(key.name)) {
          const step = key.name === "up" ? -1 : 1;
          let next = index;
          for (let count = 0; count < entries.length; count++) {
            next = (next + step + entries.length) % entries.length;
            if (!entries[next].disabled) { index = next; break; }
          }
        } else if (multiple && key.name === "space" && index >= 0) {
          const value = entries[index].value;
          if (selected.has(value)) selected.delete(value); else selected.add(value);
        } else if (multiple && key.name === "a" && !key.ctrl) {
          const values = entries.filter(entry => !entry.disabled).map(entry => entry.value);
          if (values.every(value => selected.has(value))) selected.clear(); else values.forEach(value => selected.add(value));
        } else if (key.name === "return") {
          if (multiple && selected.size === 0) error = zh ? "请至少选择一项，或按 Esc 返回。" : "Select at least one item, or press Esc to go back.";
          else if (multiple) { finish(entries.filter(entry => selected.has(entry.value)).map(entry => entry.value)); return; }
          else if (index >= 0) { finish(entries[index].value); return; }
        } else return;
        if (multiple) onChange([...selected]);
        draw();
      };
      draw();
    });
  }
  return {
    get cancelled() { return closed; },
    select: (message, entries, options) => choose(message, entries, options),
    multiselect: (message, entries, options) => choose(message, entries, { ...options, multiple: true }),
    question(message, { initial = "", validate = () => null, onChange = () => {} } = {}) {
      return prompt(() => {
        let value = initial;
        output.write(message + "\n> " + value);
        pending.key = (text, key) => {
          if (key.name === "escape") { finish(BACK); return; }
          if (key.name === "return") {
            const error = validate(value);
            if (!error) { finish(value); return; }
            output.write(`\n${error}\n> ${value}`);
          } else if (key.name === "backspace") {
            const chars = Array.from(value);
            chars.pop(); value = chars.join(""); onChange(value);
            output.write(cursor ? `\r\u001b[2K> ${fitLine(value, Math.max(16, (output.columns ?? 80) - 3))}` : `\n> ${value}`);
          } else if (text && !key.ctrl && !key.meta && !/[\u0000-\u001f\u007f-\u009f]/u.test(text)) {
            value += text; onChange(value);
            output.write(cursor ? `\r\u001b[2K> ${fitLine(value, Math.max(16, (output.columns ?? 80) - 3))}` : text);
          }
        };
      });
    },
    close() {
      closed = true;
      finish(CANCEL);
      input.off("keypress", onKey); input.off("end", onEnd); input.off("error", onEnd);
    },
  };
}

/**
 * Prompt rows occupy one physical line, including CJK labels in narrow terminals.
 * Control characters in external version/error text never become terminal commands.
 */
function fitLine(value, width) {
  let result = "", cells = 0;
  for (const char of String(value).replace(/[\u0000-\u001f\u007f-\u009f]/gu, " ")) {
    const point = char.codePointAt(0);
    const size = /\p{Mark}/u.test(char) ? 0 : point >= 0x1100 && (point <= 0x115f || point >= 0x2e80 && point <= 0xa4cf || point >= 0xac00 && point <= 0xd7a3 || point >= 0xf900 && point <= 0xfaff || point >= 0xfe10 && point <= 0xfe6f || point >= 0xff01 && point <= 0xff60 || point >= 0x1f300) ? 2 : 1;
    if (cells + size > width - 1) return result + "…";
    result += char; cells += size;
  }
  return result;
}
