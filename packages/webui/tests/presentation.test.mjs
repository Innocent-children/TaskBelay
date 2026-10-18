import assert from "node:assert/strict";
import test from "node:test";
import {
  applicability,
  duration,
  repositoryName,
  taskTitle,
} from "../src/lib/presentation.ts";
const en = (english) => english;

test("task labels shorten presentation without losing the original request or Windows repository names", () => {
  const request =
    "A long request with a second sentence.\nThe original text is still available in details.";
  assert.equal(taskTitle(request, 8), "A long r…");
  assert.equal(
    taskTitle("重做任务详情与检查记录的布局，保留所有现有操作"),
    "重做任务详情与检查记录的布局",
  );
  assert.equal(repositoryName("C:\\work\\my-project\\"), "my-project");
  assert.equal(repositoryName("/work/my-project/"), "my-project");
});

test("saved passing results never claim current applicability until Core refreshes conditions", () => {
  const check = { evidence_id: "record", applicable: true };
  assert.equal(
    applicability(check, { status: "active", refreshed: false }, en),
    "Current code not inspected",
  );
  assert.equal(
    applicability(check, { status: "active", refreshed: true }, en),
    "Applies to inspected conditions",
  );
  assert.equal(
    applicability(
      { ...check, applicable: false },
      { status: "active", refreshed: true },
      en,
    ),
    "Does not apply to current conditions",
  );
  assert.equal(
    applicability(check, { status: "done", refreshed: true }, en),
    "Saved at task end",
  );
  assert.equal(
    applicability(
      { evidence_id: null },
      { status: "active", refreshed: true },
      en,
    ),
    "No recorded result",
  );
});

test("unknown usage stays unknown and long command time is readable", () => {
  assert.equal(duration(null, en), "Unknown");
  assert.equal(duration(0, en), "0 ms");
  assert.equal(duration(332900, en), "5m 33s");
  assert.equal(duration(59999, en), "60.0 s");
});
