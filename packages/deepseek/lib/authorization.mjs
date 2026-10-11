import { realpathSync } from "node:fs";
import { isAbsolute, relative, sep } from "node:path";

import {
  TASKBELAY_QUALIFIED_TOOL_NAMES,
  isTaskBelayNamespaceTool,
  isExpectedTaskBelayTool,
} from "./tool-names.mjs";

export const DENIAL_CODES = Object.freeze({
  SELECTOR_REQUIRED: "TASKBELAY_SELECTOR_REQUIRED",
  UNEXPECTED_TOOL: "TASKBELAY_UNEXPECTED_TOOL",
  NO_AGENT: "TASKBELAY_NO_AGENT",
  NO_OPEN_TURN: "TASKBELAY_NO_OPEN_TURN",
  REPOSITORY_PATH_INVALID: "TASKBELAY_REPOSITORY_PATH_INVALID",
  REPOSITORY_OUTSIDE_WORKSPACE: "TASKBELAY_REPOSITORY_OUTSIDE_WORKSPACE",
});

const selectorPattern = /(^|\s)\/taskbelay(?=\s|$)/u;
const selectorInstruction = "include a whitespace-bounded /taskbelay in the current direct user turn";

export function hasDirectUserSelector(message) {
  if (message?.source?.kind !== "user" || !Array.isArray(message.content)) return false;
  return message.content.some(
    (block) => block?.type === "text"
      && typeof block.text === "string"
      && selectorPattern.test(block.text),
  );
}

export function deriveCurrentTurn(execution) {
  try {
    const agent = execution?.agent;
    if (agent?.status !== "running") return undefined;
    const events = agent.session.snapshotEvents();
    const matchingCalls = events
      .map((event, index) => ({ event, index }))
      .filter(({ event }) => event?.type === "tool/call"
        && String(event.data?.callId) === String(execution.callId));

    if (matchingCalls.length === 1) {
      return deriveDurableCallTurn(execution, events, matchingCalls[0]);
    }
    if (matchingCalls.length > 1 || execution.parent === undefined) return undefined;
    return deriveNestedTurn(events);
  } catch {
    return undefined;
  }
}

export function currentDirectUserText(execution) {
  const turn = deriveCurrentTurn(execution);
  if (turn === undefined) return "";
  const ids = new Set(turn.directUserMessageIds);
  return execution.agent.session.snapshotEvents()
    .filter((event) => event?.type === "user/message"
      && ids.has(event.data?.id)
      && event.data?.source?.kind === "user")
    .flatMap((event) => event.data.content ?? [])
    .filter((block) => block?.type === "text" && typeof block.text === "string")
    .map((block) => block.text)
    .join("\n");
}

export function authorizeTaskBelayExecution(execution, {
  workspaceRoot = process.cwd(),
  realpathImpl = realpathSync,
} = {}) {
  if (!isTaskBelayNamespaceTool(execution?.name)) return undefined;
  if (!isExpectedTaskBelayTool(execution.name)) {
    return `${DENIAL_CODES.UNEXPECTED_TOOL}: the TaskBelay namespace permits only the contracted Core tools.`;
  }
  if (execution.agent === undefined) {
    return `${DENIAL_CODES.NO_AGENT}: ${selectorInstruction}.`;
  }

  const turn = deriveCurrentTurn(execution);
  if (turn === undefined) {
    return `${DENIAL_CODES.NO_OPEN_TURN}: ${selectorInstruction}.`;
  }
  if (execution.name === TASKBELAY_QUALIFIED_TOOL_NAMES[1]) {
    return authorizeRepositoryScope(execution.arguments, { workspaceRoot, realpathImpl });
  }
  return undefined;
}

export function registerTaskBelayGuard(ctx, {
  workspaceRoot = process.cwd(),
  realpathImpl = realpathSync,
} = {}) {
  let canonicalWorkspaceRoot;
  try {
    canonicalWorkspaceRoot = realpathImpl(workspaceRoot);
  } catch {
    canonicalWorkspaceRoot = null;
  }
  return ctx.tools.guard((execution) => authorizeTaskBelayExecution(execution, {
    workspaceRoot: canonicalWorkspaceRoot ?? workspaceRoot,
    realpathImpl,
  }));
}

function authorizeRepositoryScope(arguments_, { workspaceRoot, realpathImpl }) {
  if (!isPlainObject(arguments_)) {
    return `${DENIAL_CODES.REPOSITORY_PATH_INVALID}: open-task arguments must be an object.`;
  }
  let canonicalRoot;
  try {
    canonicalRoot = realpathImpl(workspaceRoot);
  } catch {
    return `${DENIAL_CODES.REPOSITORY_PATH_INVALID}: Workspace Root is not accessible.`;
  }

  if (!Array.isArray(arguments_.repositories) || arguments_.repositories.length === 0) return "TASKBELAY_REPOSITORY_PATH_INVALID: repositories are required.";
  const declared = arguments_.repositories.map(entry => ({key:entry.key ?? "primary",path:entry.repository_path}));

  for (const repository of declared) {
    if (typeof repository.key !== "string" || repository.key.length === 0 || typeof repository.path !== "string" || !isAbsolute(repository.path)) {
      return `${DENIAL_CODES.REPOSITORY_PATH_INVALID}: repository "${safeRepositoryKey(repository.key)}" requires an absolute path.`;
    }
    let canonicalPath;
    try {
      canonicalPath = realpathImpl(repository.path);
    } catch {
      return `${DENIAL_CODES.REPOSITORY_PATH_INVALID}: repository "${safeRepositoryKey(repository.key)}" is not accessible.`;
    }
    const fromRoot = relative(canonicalRoot, canonicalPath);
    if (fromRoot === ".." || fromRoot.startsWith(`..${sep}`) || isAbsolute(fromRoot)) {
      return `${DENIAL_CODES.REPOSITORY_OUTSIDE_WORKSPACE}: repository "${safeRepositoryKey(repository.key)}" is outside the Workspace Root.`;
    }
  }
  return undefined;
}

function safeRepositoryKey(value) {
  return typeof value === "string" && /^[a-z0-9][a-z0-9._-]{0,127}$/u.test(value) ? value : "unknown";
}

function isPlainObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function deriveDurableCallTurn(execution, events, matchingCall) {
  const { event: callEvent, index: callIndex } = matchingCall;
  if (callEvent.data?.name !== execution.name) return undefined;
  const turn = callEvent.data?.turn;
  if (!Number.isInteger(turn)) return undefined;

  const starts = events
    .slice(0, callIndex + 1)
    .map((event, index) => ({ event, index }))
    .filter(({ event }) => event?.type === "turn/start" && event.data?.turn === turn);
  if (starts.length !== 1) return undefined;
  const start = starts[0];

  for (let index = start.index + 1; index < events.length; index += 1) {
    const event = events[index];
    if (event?.type === "turn/start") return undefined;
    if (event?.type === "turn/end" && event.data?.turn === turn) return undefined;
  }
  return projectTurn(events, start, callIndex, turn, callEvent.seq);
}

function deriveNestedTurn(events) {
  let open;
  for (let index = 0; index < events.length; index += 1) {
    const event = events[index];
    if (event?.type === "turn/start") {
      if (open !== undefined || !Number.isInteger(event.data?.turn)) return undefined;
      open = { event, index };
    } else if (event?.type === "turn/end") {
      if (open === undefined || event.data?.turn !== open.event.data?.turn) return undefined;
      open = undefined;
    }
  }
  if (open === undefined) return undefined;
  return projectTurn(events, open, events.length, open.event.data.turn, undefined);
}

function projectTurn(events, start, endIndex, turn, callSeq) {
  const messages = events
    .slice(start.index + 1, endIndex)
    .filter((event) => event?.type === "user/message" && event.data?.source?.kind === "user")
    .map((event) => event.data);
  return Object.freeze({
    turn,
    startSeq: start.event.seq,
    ...(callSeq === undefined ? {} : { callSeq }),
    directUserMessageIds: Object.freeze(messages.map((message) => message.id)),
    selectorPresent: messages.some(hasDirectUserSelector),
  });
}
