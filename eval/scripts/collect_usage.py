"""Recompute observed usage without turning partial tracing into a complete cost."""

from collections import Counter, defaultdict
import json
import math
from pathlib import Path

import dataset
import collect_invocations
import collect_phase_costs
import collect_feedback
from run_agents import parse_events
import trace_command


TOKEN_FIELDS = {
    "claude": ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens"),
    "codex": ("input_tokens", "cached_input_tokens", "output_tokens"),
}


def nonnegative(value):
    return type(value) is int and value >= 0 or type(value) is float and math.isfinite(value) and value >= 0


def total(values):
    if not all(nonnegative(value) for value in values):
        return None
    value = sum(values)
    return value if nonnegative(value) else None


def tokens(turns, agent):
    fields = TOKEN_FIELDS[agent]
    totals, unknown = {}, []
    usages = [turn["events"]["usage"] for turn in turns]
    for field in fields:
        values = []
        for index, reports in enumerate(usages, 1):
            # Only one terminal usage record is authoritative per invocation.
            # Assistant-message and nested iteration counters may overlap it.
            value = reports[0].get(field) if len(reports) == 1 and isinstance(reports[0], dict) else None
            if type(value) is not int or value < 0:
                unknown.append({"turn": index, "field": field})
            else:
                values.append(value)
        totals[field] = sum(values) if turns and len(values) == len(turns) else None
    return {"native_field_totals": totals, "missing_or_invalid": unknown,
            "scope": "terminal counters per invocation; cache fields are separate, not added to a synthetic total"}


def resources(calls):
    elapsed, user, system, peaks = [], [], [], []
    for call in calls:
        end = call["exit"]
        elapsed.append(end.get("elapsed_seconds") if end else None)
        usage = end.get("resources") if end else None
        usage = usage if isinstance(usage, dict) else {}
        user.append(usage.get("user_seconds"))
        system.append(usage.get("system_seconds"))
        peak, unit = usage.get("reported_maxrss"), usage.get("maxrss_unit")
        peaks.append(peak * (1024 if unit == "KiB" else 1)
                     if nonnegative(peak) and unit in {"bytes", "KiB"} else None)
    return {"inclusive_elapsed_seconds_sum": total(elapsed), "reported_user_seconds_sum": total(user),
            "reported_system_seconds_sum": total(system),
            "largest_reported_maxrss_bytes": max(peaks, default=0)
            if all(nonnegative(peak) for peak in peaks) else None}


def traced_usage(path, recorded):
    result = {"status": "unknown", "observed_native_starts": None, "origins": {}, "errors": []}
    try:
        if recorded is not None and not isinstance(recorded, dict):
            raise ValueError("command trace receipt must be an object")
        if not recorded or recorded.get("status") == "not_requested":
            raise ValueError("command tracing was not requested")
        name = recorded["file"]
        if not isinstance(name, str) or Path(name).name != name:
            raise ValueError("invalid command trace evidence path")
        evidence = path / name
        digest = dataset.digest(evidence)
        if digest != recorded["sha256"]:
            raise ValueError("command trace bytes differ from receipt")
        parsed = trace_command.collect(evidence)
        if dataset.digest(evidence) != digest:
            raise ValueError("command trace changed during usage collection")
        if parsed != recorded["records"]:
            raise ValueError("command trace summary differs from native records")
        result["evidence"] = {"file": name, "sha256": digest}
        calls = {call["id"]: call for call in parsed["calls"]}
        grouped = defaultdict(list)
        for call in calls.values():
            origin = call["attempt"]["origin"]
            if origin not in {"agent", "taskbelay", "assessment"}:
                raise ValueError("unknown trace origin")
            seen, current = {call["id"]}, call
            while current["attempt"]["parent_id"]:
                parent = current["attempt"]["parent_id"]
                if parent in seen:
                    raise ValueError("cyclic command trace ancestry")
                seen.add(parent)
                current = calls[parent]
                if call["start"] is not None and current["start"] is None:
                    raise ValueError("started child has no started ancestor")
            grouped[origin].append(call)
        for origin, items in sorted(grouped.items()):
            started = [call for call in items if call["start"] is not None]
            roots = [call for call in started if not call["attempt"]["parent_id"]]
            result["origins"][origin] = {
                "attempts": len(items), "observed_native_starts": len(started),
                "not_started": sum(call["state"] == "not_started" for call in items),
                "incomplete": sum(call["state"] == "incomplete" for call in items),
                "observed_exit_codes": dict(Counter(str(call["exit"]["returncode"]) for call in items
                                                     if call["exit"] and type(call["exit"].get("returncode")) is int)),
                "root_call_ids": [call["id"] for call in roots],
                "nested_call_ids": [call["id"] for call in started if call["attempt"]["parent_id"]],
                "root_inclusive_resources": resources(roots),
            }
        result.update(status="recorded" if parsed["complete"] else "partial",
                      observed_native_starts=parsed["observed_starts"])
        if not parsed["complete"]:
            result["errors"].append("trace has no calls or contains incomplete executions")
    except (OSError, ValueError, KeyError, TypeError) as error:
        result.update(status="unknown", observed_native_starts=None, origins={})
        result["errors"].append(str(error))
    result["scope"] = (
        "selected forwarded native commands only; resources include each root's reported descendants, "
        "so nested calls are not added again; root-origin groups do not isolate costs of other-origin descendants; "
        "elapsed sums are not elapsed wall time, and the largest individual maxrss is not concurrent peak memory")
    return result


def collect(agent_directory, metadata, receipt, classifier=None, feedback_renderer=None):
    root = Path(agent_directory)
    result = {"valid_evidence": False, "invocation_wall_seconds": None, "model_tokens": None,
              "command_trace": None, "agent_verification_invocations": None,
              "taskbelay_phase_costs": {phase: None for phase in ("prepare", "compile", "baseline", "mutation")},
              "taskbelay_feedback": None, "user_confirmations": None, "full_cost_complete": False,
              "errors": [], "limitations": [
                  "native command starts are not verification tool invocations; a call may start multiple commands",
                  "unforwarded commands and internal processes are not covered by selected-tool tracing",
                  "TaskBelay phase costs, feedback and user decisions require their own recorded evidence",
                  "Agent invocation wall time already includes concurrent hooks and MCP lifetime; do not add them again",
              ]}
    try:
        inputs = {name: dataset.digest(root / name) for name in ("metadata.json", "result.json")}
        if metadata != json.loads((root / "metadata.json").read_text()) or receipt != json.loads((root / "result.json").read_text()):
            raise ValueError("Agent metadata or receipt changed during usage collection")
        turns = receipt["turns"]
        if not turns or metadata["agent"] not in TOKEN_FIELDS:
            raise ValueError("no supported Agent invocations")
        outputs = {}
        for number, turn in enumerate(turns, 1):
            path = root / f"turn-{number}.jsonl"
            digest = dataset.digest(path)
            if turn["turn"] != number or parse_events(path, metadata["agent"]) != turn["events"]:
                raise ValueError("Agent usage receipt differs from native output")
            if dataset.digest(path) != digest:
                raise ValueError("native Agent output changed during usage collection")
            if not nonnegative(turn["duration_seconds"]):
                raise ValueError("invalid invocation wall duration")
            outputs[path.name] = digest
        traced = traced_usage(root, receipt.get("command_trace"))
        invocations = collect_invocations.collect(root, metadata["agent"], receipt, classifier)
        phase_costs = collect_phase_costs.collect(root, receipt.get("phase_checks"), metadata.get("workspace"))
        feedback = collect_feedback.collect(metadata, receipt, feedback_renderer)
        if any(dataset.digest(root / name) != digest for name, digest in {**inputs, **outputs}.items()):
            raise ValueError("Agent metadata, receipt or native output changed during usage collection")
        result.update(valid_evidence=True, invocation_wall_seconds=total([turn["duration_seconds"] for turn in turns]),
                      model_tokens=tokens(turns, metadata["agent"]),
                      command_trace=traced, verification_invocations=invocations,
                      taskbelay_phase_costs=phase_costs,
                      taskbelay_feedback=feedback if feedback_renderer is not None else None,
                      source={"metadata_sha256": inputs["metadata.json"],
                              "result_sha256": inputs["result.json"], "native_outputs_sha256": outputs,
                              "collector_sha256": dataset.digest(Path(__file__)),
                              "trace_parser_sha256": dataset.digest(Path(trace_command.__file__))})
    except (OSError, ValueError, KeyError, TypeError) as error:
        result["errors"].append(str(error))
    return result
