"""Classify retained baseline failures without rerunning unchanged native tests."""

import argparse
import copy
import json
from pathlib import Path

import dataset
from run_agents import save_json
from validate_dataset import baseline_outcome, passed


def without_failure_classification(task):
    task = copy.deepcopy(task)
    for key in ("baseline_failure_patterns", "baseline_failure_exit_codes"):
        task["checks"].pop(key, None)
    return task


def reassess(task_directory, original_result, output):
    task_directory = Path(task_directory).resolve(strict=True)
    original_result = Path(original_result).resolve(strict=True)
    root = original_result.parent
    current = dataset.read_task(task_directory)
    archived = dataset.read_task(root / "task-material")
    report = json.loads(original_result.read_text(encoding="utf-8"))
    if report["task_manifest_sha256"] != dataset.digest(root / "task-material" / "task.json"):
        raise ValueError("archived task manifest does not match the executed input")
    if without_failure_classification(current) != without_failure_classification(archived):
        raise ValueError("execution input changed; run a new validation")
    groups = [(root / "versions", report["versions"])]
    for variant, phases in report["variants"].items():
        for phase in ("prepare", "versions", "existing", "hidden"):
            groups.append((root / variant / phase, phases.get(phase, [])))
    for directory, results in groups:
        for result in results:
            for field in ("stdout", "stderr"):
                path = directory / result[field]
                if path.resolve().parent != directory or dataset.digest(path) != result[field + "_sha256"]:
                    raise ValueError("recorded command output changed")
    base, reference = report["variants"]["base"], report["variants"]["reference"]
    outcome = baseline_outcome(base.get("hidden", []), root / "base" / "hidden", current["checks"])
    expected = current["checks"].get("baseline_hidden", "fail")
    accepted = (not report.get("error") and outcome == expected
        and passed(base.get("existing", [])) and passed(reference.get("existing", []))
        and passed(reference.get("hidden", [])) and passed(report["versions"])
        and passed(base.get("versions", [])) and passed(reference.get("versions", []))
        and not base.get("preparation_failed") and not reference.get("preparation_failed"))
    source_reference = original_result.name if Path(output).resolve().parent == root else str(original_result)
    result = {"task_id": current["id"], "source_result": source_reference,
              "source_result_sha256": dataset.digest(original_result),
              "task_manifest_sha256": dataset.digest(task_directory / "task.json"),
              "current_manifest": current, "script_sha256": dataset.digest(Path(__file__)),
              "baseline_hidden_outcome": outcome, "validated": bool(accepted),
              "reason": "Only baseline failure classification changed; native outputs were verified and reused."}
    save_json(Path(output), result)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("task_directory", type=Path)
    parser.add_argument("original_result", type=Path)
    parser.add_argument("new_output", type=Path)
    arguments = parser.parse_args()
    result = reassess(arguments.task_directory, arguments.original_result, arguments.new_output)
    print(json.dumps({"task": result["task_id"], "validated": result["validated"]}))
    raise SystemExit(0 if result["validated"] else 1)


if __name__ == "__main__":
    main()
