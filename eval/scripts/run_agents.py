"""Run one evaluation scenario through a real Agent CLI and retain its evidence."""

import argparse
from contextlib import closing
import hashlib
import json
import os
from pathlib import Path
import platform
import signal
import sqlite3
import subprocess
import time

import trace_command
import collect_phase_costs


def save_json(path, value):
    with path.open("x", encoding="utf-8") as stream:
        os.chmod(path, 0o600)
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write("\n")


def command(config, session):
    agent = config["agent"]
    executable = config.get("executable", agent)
    arguments = config.get("arguments", [])
    if agent == "codex":
        if session:
            return [executable, "exec", "resume", "--json", "--model", config["model"],
                    "-c", 'sandbox_mode="workspace-write"', *arguments, session, "-"]
        return [executable, "exec", "--json", "--model", config["model"],
                "--sandbox", "workspace-write", *arguments, "-"]
    result = [executable, "-p", "--verbose", "--output-format", "stream-json",
              "--model", config["model"], *arguments]
    if session:
        result.extend(["--resume", session])
    return result


def validate(config):
    if config.get("agent") not in {"claude", "codex"}:
        raise ValueError("agent must be claude or codex")
    if not isinstance(config.get("model"), str) or not config["model"].strip():
        raise ValueError("a fixed model is required")
    if not isinstance(config.get("enabled"), bool):
        raise ValueError("enabled must explicitly select the comparison mode")
    workspace = Path(config["workspace"]).resolve(strict=True)
    if not workspace.is_dir() or not (workspace / ".git").exists():
        raise ValueError("workspace must be a prepared evaluation Git checkout")
    turns = config.get("turns")
    if not isinstance(turns, list) or not turns or any(
            not isinstance(turn, dict) or not isinstance(turn.get("prompt"), str)
            or not turn["prompt"].strip() for turn in turns):
        raise ValueError("turns must contain nonempty scripted prompts")
    if not 1 <= config.get("timeout_seconds", 600) <= 3600:
        raise ValueError("timeout_seconds must be between 1 and 3600")
    if not config.get("database"):
        raise ValueError("database is required to verify both enabled and disabled runs")
    if config.get("command_trace") is not None:
        trace = Path(config["command_trace"]).resolve()
        if trace.is_relative_to(workspace) or trace.exists() or not trace.parent.is_dir():
            raise ValueError("command_trace must be new, outside the workspace, with an existing parent")
    arguments = config.get("arguments", [])
    if not isinstance(arguments, list) or any(not isinstance(arg, str) for arg in arguments):
        raise ValueError("arguments must be an argv array")
    owned = {"--json", "--model", "-m", "--resume", "-r", "--last", "--continue",
             "--fork-session", "--session-id", "--ephemeral", "--no-session-persistence",
             "--output-format", "--sandbox", "-s", "--dangerously-skip-permissions",
             "--dangerously-bypass-approvals-and-sandbox"}
    if any(arg.split("=", 1)[0] in owned or arg == "bypassPermissions" for arg in arguments):
        raise ValueError("arguments cannot replace session, model, output or permission boundaries")
    for index, arg in enumerate(arguments):
        override = arguments[index + 1] if arg in {"-c", "--config"} and index + 1 < len(arguments) else ""
        if arg.startswith("--config="):
            override = arg[len("--config="):]
        if override.split("=", 1)[0].strip() in {"model", "sandbox_mode"}:
            raise ValueError("config overrides cannot replace the fixed model or sandbox")
    return workspace


def parse_events(path, agent):
    summary = {"session_id": None, "tool_events": 0, "completed": False,
               "errors": [], "usage": [], "models": []}
    with path.open(encoding="utf-8") as stream:
        for number, line in enumerate(stream, 1):
            if not line.strip():
                continue
            try:
                event = json.loads(line)
                if not isinstance(event, dict):
                    raise ValueError("event is not an object")
            except ValueError as error:
                summary["errors"].append(f"invalid JSON event at line {number}: {error}")
                continue
            if agent == "codex":
                if event.get("type") == "thread.started":
                    session = event.get("thread_id")
                    if summary["session_id"] not in (None, session):
                        summary["errors"].append("session changed within one invocation")
                    summary["session_id"] = session
                item = event.get("item", {})
                if not isinstance(item, dict):
                    summary["errors"].append(f"invalid item at line {number}")
                    continue
                if event.get("type") == "item.completed" and item.get("type") in {
                        "command_execution", "file_change", "mcp_tool_call"}:
                    summary["tool_events"] += 1
                if event.get("type") == "turn.completed":
                    summary["completed"] = True
                    summary["usage"].append(event.get("usage", {}))
                if event.get("type") in {"turn.failed", "error"}:
                    summary["errors"].append(event)
            else:
                session = event.get("session_id")
                if session:
                    if summary["session_id"] not in (None, session):
                        summary["errors"].append("session changed within one invocation")
                    summary["session_id"] = session
                if event.get("type") == "system" and event.get("subtype") == "init":
                    if event.get("model"):
                        summary["models"].append(event["model"])
                if event.get("type") == "assistant":
                    summary["tool_events"] += sum(item.get("type") == "tool_use"
                        for item in event.get("message", {}).get("content", []) if isinstance(item, dict))
                if event.get("type") == "result":
                    summary["completed"] = not event.get("is_error", False) and event.get("subtype") == "success"
                    summary["usage"].append(event.get("usage", {}))
                    if not summary["completed"]:
                        summary["errors"].append(event)
    return summary


def read_hooks(database, workspace, agent, after_id, session=None):
    path = Path(database).resolve()
    if not path.exists():
        return []
    with closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True, timeout=0.2)) as connection:
        connection.row_factory = sqlite3.Row
        rows = connection.execute(
            "SELECT h.id,h.agent,h.session_id,h.round_id,h.kind,h.tool_call_id,h.state,h.error,h.decision,w.path "
            "FROM hook_events h JOIN workspaces w ON w.id=h.workspace_id "
            "WHERE h.id>? AND h.agent=? ORDER BY h.id", (after_id, agent)).fetchall()
    selected = []
    for row in rows:
        try:
            same_workspace = os.path.samefile(row["path"], workspace)
        except OSError:
            same_workspace = False
        if same_workspace and (session is None or row["session_id"] == session):
            selected.append(dict(row))
    return selected


def integration(enabled, hooks, events, previous_session, returncode, timed_out):
    failures = []
    if returncode != 0 or timed_out or not events["completed"] or events["errors"]:
        failures.append("agent_run_failed")
    if not events["session_id"]:
        failures.append("session_identity_missing")
    if previous_session and previous_session != events["session_id"]:
        failures.append("resume_changed_session")
    if enabled:
        kinds = {row["kind"] for row in hooks if row["state"] == "recorded" and not row["error"]}
        for required in ("session_start", "user_prompt", "stop"):
            if required not in kinds:
                failures.append("missing_hook:" + required)
        if not kinds.intersection({"command_use", "edit_use", "mcp_use", "tool_end"}):
            failures.append("missing_hook:tool_activity")
        if not events["tool_events"]:
            failures.append("missing_agent_tool_activity")
        if any(row["state"] != "recorded" or row["error"] for row in hooks):
            failures.append("hook_not_recorded_successfully")
    elif hooks:
        failures.append("disabled_run_has_taskbelay_hooks")
    return failures


def execute(argv, prompt, workspace, output, errors, timeout, environment=None):
    started = time.monotonic()
    options = {"start_new_session": True} if os.name != "nt" else {
        "creationflags": subprocess.CREATE_NEW_PROCESS_GROUP}
    with output.open("xb") as stdout, errors.open("xb") as stderr:
        os.chmod(output, 0o600)
        os.chmod(errors, 0o600)
        process = subprocess.Popen(argv, cwd=workspace, stdin=subprocess.PIPE,
                                   stdout=stdout, stderr=stderr, env=environment, **options)
        timed_out = False
        try:
            process.communicate(prompt.encode("utf-8"), timeout=timeout)
        except (subprocess.TimeoutExpired, KeyboardInterrupt):
            timed_out = True
            if os.name == "nt":
                subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False)
            else:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            process.wait(timeout=10)
        return process.returncode, timed_out, time.monotonic() - started


def settings_snapshot(config):
    settings = []
    for name in config.get("settings_files", []):
        path = Path(name).resolve(strict=True)
        settings.append({"path": str(path), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()})
    return settings


def capture_trace(config, destination):
    if config.get("command_trace") is None:
        return {"status": "not_requested", "observed_starts": None}
    source = Path(config["command_trace"]).resolve()
    result = {"source": str(source), "status": "unknown", "observed_starts": None}
    try:
        data = source.read_bytes()
        evidence = destination / "command-trace.jsonl"
        with evidence.open("xb") as stream:
            os.chmod(evidence, 0o600)
            stream.write(data)
        result.update(file=evidence.name, sha256=hashlib.sha256(data).hexdigest())
        if source.read_bytes() != data:
            raise ValueError("command trace changed during collection")
        parsed = trace_command.collect(evidence)
        result["records"] = parsed
        if parsed["complete"]:
            result.update(status="recorded", observed_starts=parsed["observed_starts"])
        else:
            result["error"] = "no complete command trace was recorded"
    except (OSError, ValueError, KeyError, TypeError) as error:
        result["error"] = str(error)
    return result


def agent_version(config):
    version = subprocess.run([config.get("executable", config["agent"]), "--version"],
                             capture_output=True, text=True, timeout=15, check=True).stdout.strip()
    if not version:
        raise ValueError("Agent did not report its version")
    return version


def run(config, destination):
    workspace = validate(config)
    if destination == workspace or workspace in destination.parents:
        raise ValueError("evaluation evidence must be outside the Agent workspace")
    destination.mkdir(mode=0o700, parents=True, exist_ok=False)
    version = agent_version(config)
    settings = settings_snapshot(config)
    save_json(destination / "metadata.json", {"agent": config["agent"], "version": version,
              "model": config["model"], "arguments": config.get("arguments", []), "settings": settings,
              "enabled": config["enabled"], "workspace": str(workspace), "platform": platform.platform(),
              "python": platform.python_version(), "started_at": time.time(), "task_id": config.get("task_id"),
              "trace_environment": {key: os.environ.get(key) for key in
                  ("PATH", "GOTOOLCHAIN", "TASKBELAY_EVAL_TRACE_ORIGIN", "CLAUDE_CODE_SHELL",
                   "CLAUDE_CODE_SHELL_PREFIX")} if config.get("command_trace") else None})
    previous = read_hooks(config["database"], workspace, config["agent"], 0)
    phase_start = collect_phase_costs.begin(config["database"], workspace)
    cursor = max((row["id"] for row in previous), default=0)
    session = config.get("session_id")
    results = []
    for index, turn in enumerate(config["turns"], 1):
        if agent_version(config) != version or settings_snapshot(config) != settings:
            raise ValueError("Agent version or settings changed during the scenario")
        output, errors = destination / f"turn-{index}.jsonl", destination / f"turn-{index}.stderr"
        argv = command(config, session)
        save_json(destination / f"turn-{index}.input.json", {"argv": argv, "prompt": turn["prompt"]})
        returncode, timed_out, duration = execute(argv, turn["prompt"], workspace, output, errors,
                                                 config.get("timeout_seconds", 600))
        events = parse_events(output, config["agent"])
        observed_hooks = read_hooks(config["database"], workspace, config["agent"], cursor)
        hooks = [row for row in observed_hooks if row["session_id"] == events["session_id"]]
        failures = integration(config["enabled"], hooks, events, session, returncode, timed_out)
        unmatched = [row for row in observed_hooks if row["session_id"] != events["session_id"]]
        if unmatched:
            failures.append("hook_session_identity_unmatched")
        version_after = agent_version(config)
        settings_after = settings_snapshot(config)
        if version_after != version or settings_after != settings:
            failures.append("agent_version_or_settings_changed")
        # 不用“最近活跃 session”替代原生标识。无法对应时该样本无效。
        result = {"turn": index, "session_id": events["session_id"], "events": events, "hooks": hooks,
                  "version": version, "requested_model": config["model"], "settings": settings,
                  "version_after": version_after, "settings_after": settings_after,
                  "returncode": returncode, "timed_out": timed_out, "duration_seconds": duration,
                  "valid_integration": not failures, "failures": failures, "unmatched_hooks": unmatched}
        results.append(result)
        save_json(destination / f"turn-{index}.result.json", result)
        if failures:
            break
        session = events["session_id"]
        cursor = max((row["id"] for row in hooks), default=cursor)
    report = {"valid_integration": len(results) == len(config["turns"]) and all(
        result["valid_integration"] for result in results), "turns": results,
        "command_trace": capture_trace(config, destination),
        "phase_checks": collect_phase_costs.capture(config["database"], workspace, phase_start, destination),
        "correctness_evaluated": False, "performance_comparison_evaluated": False}
    save_json(destination / "result.json", report)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    try:
        report = run(json.loads(args.manifest.read_text(encoding="utf-8")), args.output.resolve())
    except (OSError, ValueError, sqlite3.Error, subprocess.SubprocessError) as error:
        parser.exit(2, f"evaluation failed: {error}\n")
    print(json.dumps({"valid_integration": report["valid_integration"], "output": str(args.output)}))
    return 0 if report["valid_integration"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
