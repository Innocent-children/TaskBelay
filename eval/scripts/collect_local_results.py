"""Summarize go test -json evidence without treating skipped tests as passes."""

import argparse
from collections import defaultdict
import hashlib
import json
from pathlib import Path
import platform


def workflow_metrics(paths):
    measurements = defaultdict(list)
    for path in paths:
        outputs, results = defaultdict(list), defaultdict(list)
        for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            event = json.loads(line)
            key = (event.get("Package", ""), event.get("Test", ""))
            if event.get("Action") == "output":
                outputs[key].append((number, event.get("Output", "")))
            elif event.get("Action") in {"pass", "fail", "skip"}:
                results[key].append(event["Action"])
        for key, chunks in outputs.items():
            steps, faults, summary = [], [], None
            for line in "".join(text for _, text in chunks).splitlines():
                if "TASKBELAY_WORKFLOW_EVENT " in line:
                    steps.append(json.loads(line.split("TASKBELAY_WORKFLOW_EVENT ", 1)[1]))
                if "TASKBELAY_WORKFLOW_FAULT " in line:
                    faults.append(json.loads(line.split("TASKBELAY_WORKFLOW_FAULT ", 1)[1]))
                if "TASKBELAY_WORKFLOW_METRICS " in line:
                    if summary is not None:
                        raise ValueError("duplicate workflow summary: " + key[1])
                    summary = json.loads(line.split("TASKBELAY_WORKFLOW_METRICS ", 1)[1])
            if summary is None:
                continue
            if not steps or [row["event"] for row in steps] != list(range(1, len(steps) + 1)):
                raise ValueError("missing or repeated workflow events: " + key[1])
            computed = {"events": len(steps), "extra_approvals": 0, "duplicate_prompts": 0, "invalid_blocks": 0}
            first_blocks = set()
            for row in steps:
                if row["id"] != summary["id"]:
                    raise ValueError("workflow identity changed: " + key[1])
                decision = row["decision"]
                blocked = bool(decision.get("DenyTool") or decision.get("BlockStop"))
                text = "".join(decision.values())
                scope_s1, scope_s2 = "[S1" in text, "[S2" in text
                permitted = (not decision.get("BlockStop") and not scope_s2 and (
                    row["allowed_block"] == "stale" and "过期" in decision.get("DenyTool", "")
                    or row["allowed_block"] == "S1" and decision.get("DenyTool", "").startswith("[S1]")))
                if row["allowed_block"].startswith("D1:"):
                    issue_id = row["allowed_block"][3:]
                    block = decision.get("BlockStop", "")
                    permitted = (bool(issue_id) and issue_id not in first_blocks and not decision.get("DenyTool")
                        and not scope_s1 and not scope_s2 and block.startswith("[D1 ")
                        and block.count("编号=") == 1 and "编号=" + issue_id + "\n" in block)
                    first_blocks.add(issue_id)
                actual = {"invalid_block": blocked and not permitted,
                          "extra_approval": scope_s2 or scope_s1 and row["allowed_block"] != "S1",
                          "duplicate_prompt": bool(row["expected_quiet"] and any(decision.values()))}
                if any(row[name] != value for name, value in actual.items()):
                    raise ValueError("workflow counts disagree with output: " + key[1])
                for field, name in [("invalid_block", "invalid_blocks"), ("extra_approval", "extra_approvals"),
                                    ("duplicate_prompt", "duplicate_prompts")]:
                    computed[name] += int(actual[field])
            if any(summary[name] != value for name, value in computed.items()):
                raise ValueError("incorrect workflow aggregate: " + key[1])
            complete = (summary.get("complete") is True and results[key] == ["pass"]
                        and not any(row.get("error") for row in steps))
            if any(row.get("id") != summary["id"] or not row.get("error")
                   or row.get("delivered") is not False or row.get("summary_version_unchanged") is not True
                   for row in faults):
                raise ValueError("incomplete workflow fault evidence: " + key[1])
            measurements[summary["id"]].append({**summary, "complete": complete, "test": key[1],
                "package": key[0], "source": str(path), "first_output_line": chunks[0][0], "steps": steps, "faults": faults})
    return measurements


def test_outcomes(paths):
    outcomes = {}
    package_results = []
    for path in paths:
        for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            event = json.loads(line)
            if event.get("Action") not in {"pass", "fail", "skip"}:
                continue
            outcome = {"result": event["Action"], "seconds": event.get("Elapsed"),
                       "source": str(path), "line": number}
            if event.get("Test"):
                key = (event.get("Package", ""), event["Test"])
                outcomes.setdefault(key, []).append(outcome)
            else:
                package_results.append({"package": event.get("Package"), **outcome})
    return outcomes, package_results


def collect(paths, coverage, cases, native_paths=()):
    outcomes, package_results = test_outcomes(paths)
    native_outcomes, native_packages = test_outcomes(native_paths)
    fixtures = []
    for ledger in sorted(cases.rglob("ledger.yaml")):
        directory = ledger.parent
        identifier = directory.relative_to(cases).as_posix()
        for required in ["base", "change.patch", "expected.yaml", "README.md"]:
            if not (directory / required).exists():
                raise ValueError(f"{identifier}: missing {required}")
        matched = [result for (_, name), results in outcomes.items()
                   if name == "TestCasePacks/" + identifier for result in results]
        hashes = {file.relative_to(directory).as_posix(): hashlib.sha256(file.read_bytes()).hexdigest()
                  for file in sorted(directory.rglob("*")) if file.is_file()
                  and not any(parent.name == "__pycache__" for parent in file.parents)
                  and (file.parent == directory or directory / "base" in file.parents)}
        fixtures.append({"id": identifier, "outcomes": matched, "files_sha256": hashes,
                         "result": status(matched)})
    def group(item, evidence=None):
        if evidence is None:
            evidence = outcomes
        results = []
        missing = []
        for name in item["tests"]:
            matched = [result for (_, test), attempts in evidence.items() if test == name for result in attempts]
            if not matched:
                missing.append(name)
            results.extend({"test": name, **result} for result in matched)
        result = status(results)
        if result != "fail":
            if missing:
                result = "not_run"
            elif item.get("gaps") and result == "pass":
                result = "partial"
        return {**item, "result": result, "missing_tests": missing, "outcomes": results}

    measured = workflow_metrics(paths)
    workflows = []
    for item in coverage["workflows"]:
        attempts = [row for row in measured[item["id"]] if row["test"] == item.get("metrics_test")]
        counters = {"extra_approvals": None, "duplicate_prompts": None, "invalid_blocks": None}
        if attempts and all(row["complete"] for row in attempts):
            counters = {name: sum(row[name] for row in attempts) for name in counters}
        workflows.append({**group(item), **counters, "measurement_attempts": attempts,
                          "counter_scope": "scripted hook events; each completed attempt counted separately"})
    rules = [group(item) for item in coverage.get("rules", [])]
    rule_index = {item["id"]: item for item in rules}
    languages = []
    for item in coverage.get("languages", []):
        facts = [group(part) for part in item["facts"]]
        if not facts or not item["rule_ids"]:
            raise ValueError("language requires facts and shared rules: " + item["id"])
        if any(rule not in rule_index for rule in item["rule_ids"]):
            raise ValueError("unknown language rule: " + item["id"])
        shared = [{"id": rule, "result": rule_index[rule]["result"]} for rule in item["rule_ids"]]
        native = group(item["native"], native_outcomes) if item.get("native") else None
        parts = facts + shared + ([native] if native else [])
        states = {part["result"] for part in parts}
        result = "fail" if "fail" in states else "pass" if states == {"pass"} else "partial"
        languages.append({**item, "facts": facts, "shared_rules": shared, "native": native, "result": result})
    return {"platform": platform.platform(), "source_sha256": {
        str(path): hashlib.sha256(path.read_bytes()).hexdigest() for path in [*paths, *native_paths]},
        "packages": package_results, "cases": fixtures, "workflows": workflows,
        "rules": rules, "languages": languages, "native_packages": native_packages,
        "native_skipped_tests": [{"test": test, **result} for (_, test), results in native_outcomes.items()
                                 for result in results if result["result"] == "skip"],
        "skipped_tests": [{"test": test, **result} for (_, test), results in outcomes.items()
                          for result in results if result["result"] == "skip"],
        "counts": {"case_packs": len(fixtures), "passed_case_packs": sum(
            case["result"] == "pass" for case in fixtures), "language_rows": len(languages),
                   "passed_language_rows": sum(row["result"] == "pass" for row in languages)},
        "real_agent_comparison": "not_run", "product_adoption": "not_evaluated",
        "limitations": ["Local Go test evidence is not native Agent or cross-platform evidence.",
                        "Workflow counters require complete event evidence; unmeasured values remain null, not zero.",
                        "All attempts are retained; a retry does not erase an earlier failure."]}


def status(results):
    values = {result["result"] for result in results}
    if not values:
        return "not_run"
    if "fail" in values:
        return "fail"
    if "skip" in values:
        return "partial" if "pass" in values else "skip"
    return "pass"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("logs", type=Path, nargs="+")
    parser.add_argument("--native-logs", type=Path, nargs="*", default=[])
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    evaluation = Path(__file__).resolve().parents[1]
    coverage = json.loads((evaluation / "coverage.json").read_text(encoding="utf-8"))
    report = collect(args.logs, coverage, evaluation / "cases", args.native_logs)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("x", encoding="utf-8") as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    print(json.dumps(report["counts"]))


if __name__ == "__main__":
    main()
