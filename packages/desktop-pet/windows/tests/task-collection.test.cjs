const test = require("node:test");
const assert = require("node:assert/strict");
const { TaskCollection } = require("../task-collection.cjs");
const summary = (id, lifecycle = "active", updated_at = "2026-09-10T00:00:00Z") => ({ task_id: id, lifecycle, updated_at, archived: false, current_node: lifecycle === "done" ? "DONE" : "ACTIVE" });

test("all tasks remain visible; blocked preempts active while equal priority retains focus", () => {
  const tasks = new TaskCollection();
  tasks.update([summary("a"), summary("b")]);
  assert.equal(tasks.focus, "a");
  tasks.update([summary("b", "active", "2026-09-11T00:00:00Z"), summary("a"), summary("c", "blocked")]);
  assert.equal(tasks.focus, "c");
  assert.equal(tasks.cards.length, 3);
  tasks.update([summary("c", "blocked"), summary("d", "blocked", "2026-09-12T00:00:00Z")]);
  assert.equal(tasks.focus, "c");
});

test("completion holds for three seconds, then hands off while preserving unread result", () => {
  let now = 0;
  const tasks = new TaskCollection(() => now);
  tasks.update([summary("a"), summary("b")]);
  tasks.update([summary("a", "done"), summary("b")]);
  assert.equal(tasks.focus, "a");
  assert.equal(tasks.display.prompt, true);
  now = 2999; tasks.update([summary("b")]); assert.equal(tasks.focus, "a");
  now = 3000; tasks.update([summary("b")]); assert.equal(tasks.focus, "b");
  assert.equal(tasks.cards.find(c => c.taskID === "a").unread, true);
  tasks.acknowledge("a"); assert.equal(tasks.cards.length, 1);
});

test("pinning preserves a terminal task and auto mode releases it", () => {
  const tasks = new TaskCollection(); tasks.pin("a");
  tasks.update([summary("a", "done"), summary("b", "blocked")]);
  assert.equal(tasks.focus, "a"); assert.equal(tasks.display.prompt, false);
  tasks.pin(null); assert.equal(tasks.focus, "b");
});

test("disconnect preserves cards; recovery and first reads do not announce historical completion", () => {
  const tasks = new TaskCollection(); tasks.update([summary("a"), summary("b")]);
  tasks.disconnect(); assert.equal(tasks.cards.length, 2);
  assert.equal(tasks.cards[0].result.stale, true);
  tasks.update([summary("a", "done"), summary("b")]);
  assert.equal(tasks.focus, "b"); assert.equal(tasks.cards.length, 1);
  tasks.update([summary("b", "cancelled")]);
  assert.equal(tasks.focus, null); assert.equal(tasks.display.phase, "idle");
});
