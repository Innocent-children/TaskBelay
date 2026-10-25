"""Collect paired Agent correctness evidence without promoting pilots or hiding retries."""

import argparse
from collections import Counter
import hashlib
import json
import math
from pathlib import Path

import dataset
import collect_usage
from assess_candidate import TEST_RESULT_FORMATS, rust_context, test_status
from run_agents import integration, parse_events


def load_json(path):
    return json.loads(path.read_text(encoding="utf-8"))


def identity(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False).encode()).hexdigest()


def checked_file(directory, name, expected):
    path = (directory / name).resolve(strict=True)
    if not path.is_relative_to(directory) or dataset.digest(path) != expected:
        raise ValueError("evidence changed or leaves its directory: " + name)
    return path


def check_agent_receipt(agent, metadata, receipt):
    if not receipt["turns"]:
        raise ValueError("Agent receipt has no completed invocation")
    previous = None
    for number, turn in enumerate(receipt["turns"], 1):
        events = parse_events(agent / f"turn-{number}.jsonl", metadata["agent"])
        if events != turn["events"] or turn["turn"] != number:
            raise ValueError("Agent result differs from native output")
        if turn["session_id"] != events["session_id"] or any(
                hook["session_id"] != events["session_id"] for hook in turn["hooks"]):
            raise ValueError("hook and Agent session identities differ")
        failures = integration(metadata["enabled"], turn["hooks"], events, previous,
                               turn["returncode"], turn["timed_out"])
        if turn["unmatched_hooks"]:
            failures.append("hook_session_identity_unmatched")
        if turn["version_after"] != turn["version"] or turn["settings_after"] != turn["settings"]:
            failures.append("agent_version_or_settings_changed")
        if (turn["version"] != metadata["version"] or turn["settings"] != metadata["settings"]
                or turn["requested_model"] != metadata["model"] or failures != turn["failures"]
                or turn["valid_integration"] != (not failures)):
            raise ValueError("Agent integration receipt is inconsistent")
        if not math.isfinite(turn["duration_seconds"]) or turn["duration_seconds"] < 0:
            raise ValueError("invalid invocation duration")
        previous = events["session_id"]
    if receipt["valid_integration"] is True and not all(turn["valid_integration"] for turn in receipt["turns"]):
        raise ValueError("incomplete invocations marked as valid integration")


def read_attempt(entry, series, root, classifier=None, feedback_renderer=None):
    row = {key: entry[key] for key in ("id", "series", "task_id", "phase", "replicate")}
    row.update(eligible=False, integration=None, existing="unknown", hidden="unknown", errors=[])
    try:
        agent = (root / entry["agent_directory"]).resolve(strict=True)
        metadata, receipt = load_json(agent / "metadata.json"), load_json(agent / "result.json")
        row["evidence"] = {"agent_directory": str(agent),
                           "agent_metadata_sha256": dataset.digest(agent / "metadata.json"),
                           "agent_result_sha256": dataset.digest(agent / "result.json")}
        row["receipt_identity"] = identity(row["evidence"]["agent_metadata_sha256"] +
                                             row["evidence"]["agent_result_sha256"])
        if type(metadata["enabled"]) is not bool:
            raise ValueError("enabled must be a boolean in Agent metadata")
        mode = "on" if metadata["enabled"] else "off"
        row["mode"] = mode
        for field in ("agent", "version", "model", "platform"):
            if metadata[field] != series[field]:
                raise ValueError("Agent metadata differs from fixed series: " + field)
        if metadata["task_id"] != entry["task_id"]:
            raise ValueError("Agent receipt belongs to another task")
        actual = {field: metadata[field] for field in ("arguments", "settings")}
        if actual != series["configuration"][mode]:
            raise ValueError("Agent arguments or settings differ from fixed series")
        check_agent_receipt(agent, metadata, receipt)
        row["usage"] = collect_usage.collect(agent, metadata, receipt, classifier, feedback_renderer)
        row["integration"] = receipt["valid_integration"] is True
        row["integration_failures"] = [failure for turn in receipt["turns"] for failure in turn["failures"]]
        row["recorded_agent_wall_seconds"] = sum(turn["duration_seconds"] for turn in receipt["turns"])
        row["observed_models"] = sorted({model for turn in receipt["turns"] for model in turn["events"]["models"]})
        if row["observed_models"] and row["observed_models"] != [metadata["model"]]:
            raise ValueError("observed model differs from fixed model")
        if not entry.get("assessment_directory"):
            row["errors"].append("candidate assessment is missing")
            return row
        assessment = (root / entry["assessment_directory"]).resolve(strict=True)
        report = load_json(assessment / "result.json")
        row["evidence"].update(assessment_directory=str(assessment),
                               assessment_sha256=dataset.digest(assessment / "result.json"))
        for field in ("agent_metadata_sha256", "agent_result_sha256"):
            if report[field] != row["evidence"][field]:
                raise ValueError("assessment belongs to different Agent evidence: " + field)
        if report["task_id"] != entry["task_id"] or report["valid_integration"] != row["integration"]:
            raise ValueError("assessment identity or integration status is inconsistent")
        task = dataset.read_task(assessment / "task-material")
        if task["id"] != entry["task_id"] or task["language"] not in TEST_RESULT_FORMATS:
            raise ValueError("unsupported candidate assessment language")
        if task["language"] != "Go" and report.get("test_result_format") != TEST_RESULT_FORMATS[task["language"]]:
            raise ValueError("candidate requires its native test report format")
        row["task_material_sha256"] = dataset.digest(assessment / "task-material/task.json")
        checked_file(assessment, "candidate.patch", report["candidate_patch_sha256"])
        row["failures"] = {}
        for phase in ("existing", "hidden"):
            result = report[phase]
            commands = result.get("commands", [])
            for command in commands:
                for stream in ("stdout", "stderr"):
                    checked_file(assessment, phase + "/" + command[stream], command[stream + "_sha256"])
                if task["language"] == "Java":
                    for native in command["test_reports"]:
                        checked_file(assessment, phase + "/" + native["path"], native["sha256"])
                elif task["language"] not in {"Go", "Rust"}:
                    checked_file(assessment, phase + "/" + command["test_report"], command["test_report_sha256"])
            context = (rust_context(report, assessment, task, phase, row.get("existing"))
                       if task["language"] == "Rust" and commands else None)
            current = test_status(commands, assessment / phase, task["language"], context)
            if any(current[key] != result.get(key) for key in current):
                raise ValueError("test result differs from recorded raw output: " + phase)
            row[phase] = current["status"]
            identifiers = set()
            for failure in current["failures"]:
                identifier = (failure.get("Package", ""), failure.get("Test", ""))
                if task["language"] == "TypeScript":
                    identifier += (failure["kind"], failure.get("case_index", -1), failure.get("typecheck", False))
                elif task["language"] == "Rust":
                    identifier = (failure["kind"], failure["command"], failure["target"], failure.get("source", ""),
                                  failure["name"], failure.get("code") or "")
                identifiers.add(identifier)
            row["failures"][phase] = sorted(identifiers)
        row["eligible"] = (row["integration"] and report["correctness_evaluated"] is True and
                           report["valid_correctness_sample"] is True and "error" not in report and
                           all(row[phase] in {"pass", "fail"} for phase in ("existing", "hidden")))
        if not row["eligible"]:
            row["errors"].append(report.get("error", "integration or correctness assessment is incomplete"))
    except (OSError, ValueError, KeyError, TypeError) as error:
        row["eligible"] = False
        row["errors"].append(str(error))
    return row


def slots(rows, phase, mode):
    result = []
    for replicate in range(1, 4):
        attempts = [row for row in rows if row["phase"] == phase and row.get("mode") == mode
                    and row["replicate"] == replicate]
        eligible = [row for row in attempts if row["eligible"]]
        # A valid failure is a sample. Choosing a later successful retry would
        # silently change its probability of inclusion in the comparison.
        result.append({"replicate": replicate, "attempts": [row["id"] for row in attempts],
                       "status": "complete" if len(eligible) == 1 else "conflict" if eligible else "missing",
                       "selected": eligible[0]["id"] if len(eligible) == 1 else None})
    return result


def outcomes(items, by_id, phase):
    selected = [by_id[item["selected"]] for item in items if item["selected"] is not None]
    complete = len(selected) == 3
    failures = Counter(failure for row in selected for failure in row["failures"][phase])
    passes = sum(row[phase] == "pass" for row in selected)
    return {"complete": complete, "observed": len(selected), "passes": passes,
            "failures": len(selected) - passes, "pass_rate": passes / 3 if complete else None,
            "failed_tests": [{"runs": count, **(
                              {"kind": key[0], "command": key[1], "target": key[2], "source": key[3],
                               "test": key[4], "compiler_code": key[5] or None} if len(key) == 6 else
                              {"package": key[0], "test": key[1],
                               **({"kind": key[2], "case_index": key[3] if key[3] >= 0 else None,
                                   "typecheck": key[4]} if len(key) == 5 else {})})}
                             for key, count in sorted(failures.items())]}


def compare_task(task_id, rows):
    by_id = {row["id"]: row for row in rows}
    result = {"task_id": task_id, "main": {}, "followup": {}, "changes": [], "additional_runs_missing": []}
    for phase in ("main", "followup"):
        for mode in ("off", "on"):
            runs = slots(rows, phase, mode)
            result[phase][mode] = {"slots": runs, **{kind: outcomes(runs, by_id, kind)
                                                    for kind in ("existing", "hidden")}}
    for kind in ("existing", "hidden"):
        off, on = (result["main"][mode][kind] for mode in ("off", "on"))
        if off["complete"] and on["complete"] and (off["passes"] != on["passes"] or off["failed_tests"] != on["failed_tests"]):
            result["changes"].append({"tests": kind, "off_passes": off["passes"], "on_passes": on["passes"],
                                       "regression": on["passes"] < off["passes"],
                                       "failure_tests_changed": off["failed_tests"] != on["failed_tests"]})
    if result["changes"]:
        for mode in ("off", "on"):
            result["additional_runs_missing"].extend({"mode": mode, "replicate": item["replicate"],
                                                      "status": item["status"]}
                                                     for item in result["followup"][mode]["slots"]
                                                     if item["status"] != "complete")
    result["failure_review_required"] = bool(result["changes"]) or any(
        result[phase][mode][kind]["failures"] for phase in ("main", "followup")
        for mode in ("off", "on") for kind in ("existing", "hidden"))
    return result


def collect(manifest_path, classifier=None, feedback_renderer=None):
    manifest_path = Path(manifest_path).resolve(strict=True)
    manifest, root = load_json(manifest_path), manifest_path.parent
    if manifest["schema_version"] != 1:
        raise ValueError("unsupported comparison schema")
    index_path = (root / manifest["dataset"]).resolve(strict=True)
    task_ids = [task["id"] for task in load_json(index_path)["tasks"]]
    if not task_ids or len(task_ids) != len(set(task_ids)):
        raise ValueError("dataset must contain unique tasks")
    materials = {}
    for task_id in task_ids:
        if not isinstance(task_id, str) or not task_id or Path(task_id).name != task_id or task_id in {".", ".."}:
            raise ValueError("invalid dataset task identity")
        directory = index_path.parent / task_id
        if dataset.read_task(directory)["id"] != task_id:
            raise ValueError("dataset index and material identities differ")
        materials[task_id] = dataset.digest(directory / "task.json")
    series = {}
    for item in manifest["series"]:
        if item["id"] in series or item["agent"] not in {"claude", "codex"} or type(item["dynamic_checks"]) is not bool:
            raise ValueError("invalid or repeated comparison series")
        for field in ("id", "version", "model", "platform"):
            if not isinstance(item[field], str) or not item[field]:
                raise ValueError("series must fix " + field)
        if set(item["configuration"]) != {"off", "on"}:
            raise ValueError("both mode configurations must be recorded")
        series[item["id"]] = item
    if not series:
        raise ValueError("at least one fixed series is required")
    rows, ids = [], set()
    for entry in manifest["attempts"]:
        if (entry["id"] in ids or not entry["id"] or entry["series"] not in series or entry["task_id"] not in task_ids
                or entry["phase"] not in {"pilot", "main", "followup"}
                or type(entry["replicate"]) is not int or entry["replicate"] not in range(1, 4)):
            raise ValueError("invalid or repeated attempt identity")
        ids.add(entry["id"])
        rows.append(read_attempt(entry, series[entry["series"]], root, classifier, feedback_renderer))
    repeated = Counter(row["receipt_identity"] for row in rows if "receipt_identity" in row)
    for row in rows:
        if repeated.get(row.get("receipt_identity"), 0) > 1:
            row["eligible"] = False
            row["errors"].append("the same Agent evidence was submitted more than once")
        if "task_material_sha256" in row and row["task_material_sha256"] != materials[row["task_id"]]:
            row["eligible"] = False
            row["errors"].append("task material differs from the selected dataset")
    groups = []
    for series_id, item in series.items():
        attempts = [row for row in rows if row["series"] == series_id]
        tasks = [compare_task(task_id, [row for row in attempts if row["task_id"] == task_id]) for task_id in task_ids]
        phase_counts = {}
        for phase in ("pilot", "main", "followup"):
            subset = [row for row in attempts if row["phase"] == phase]
            known = [row for row in subset if row["integration"] is not None]
            failures = sum(row["integration"] is False for row in known)
            phase_counts[phase] = {"attempts": len(subset), "eligible_attempts": sum(row["eligible"] for row in subset),
                                   "integration_observed": len(known), "integration_failures": failures,
                                   "integration_failure_rate": failures / len(known) if known else None,
                                   "integration_repair_time_seconds": None}
        completed = sum(slot["status"] == "complete" for task in tasks for mode in ("off", "on")
                        for slot in task["main"][mode]["slots"])
        overall = {}
        for mode in ("off", "on"):
            overall[mode] = {}
            for kind in ("existing", "hidden"):
                selected = [task["main"][mode][kind] for task in tasks]
                passed = sum(item["passes"] for item in selected)
                overall[mode][kind] = {"observed": sum(item["observed"] for item in selected), "passes": passed,
                                       "pass_rate": passed / (len(tasks) * 3) if all(item["complete"] for item in selected) else None,
                                       "failed_tasks": [task["task_id"] for task in tasks if task["main"][mode][kind]["failures"]]}
        groups.append({**item, "dynamic_setting_evidence": "declared_in_manifest",
                       "attempt_counts": phase_counts, "required_main_runs": len(task_ids) * 6,
                       "completed_main_slots": completed, "missing_main_slots": len(task_ids) * 6 - completed,
                       "overall_main_correctness": overall, "tasks": tasks})
    return {"schema_version": 1, "source": {"manifest": str(manifest_path), "manifest_sha256": dataset.digest(manifest_path),
            "dataset": str(index_path), "dataset_sha256": dataset.digest(index_path), "task_material_sha256": materials,
            "collector_sha256": dataset.digest(Path(__file__))},
            "task_count": len(task_ids), "document_main_runs": len(task_ids) * 12,
            "unconfigured_agents": sorted({"claude", "codex"} - {item["agent"] for item in series.values()}),
            "attempts": rows, "series": groups,
            "adoption": {"status": "not_evaluated", "reasons": [
                "correctness collection alone does not establish fixed-condition equivalence or dynamic setting execution",
                "quantity metrics, full runtime cost, platform and workflow evidence, and human audit are evaluated separately"]}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--command-classifier", type=Path)
    parser.add_argument("--feedback-renderer", type=Path)
    args = parser.parse_args()
    report = collect(args.manifest, args.command_classifier, args.feedback_renderer)
    with args.output.open("x", encoding="utf-8") as output:
        json.dump(report, output, ensure_ascii=False, indent=2)
        output.write("\n")


if __name__ == "__main__":
    main()
