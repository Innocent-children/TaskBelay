"""Collect test quantities from full fixed trees, separately from correctness copies."""

import argparse
from collections import Counter, defaultdict
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import time
import xml.etree.ElementTree as ET

import assess_candidate
import dataset
from run_agents import save_json
from validate_dataset import input_fingerprint, passed, run_commands, substitute


def vitest_cases(report, root):
    files = defaultdict(lambda: {"runtime": [], "typecheck": []})
    runtime_files = {file["name"] for file in report["testResults"]
                     if any(case.get("meta", {}).get("typecheck") is not True for case in file["assertionResults"])}
    for file in report["testResults"]:
        package = Path(file["name"]).relative_to(root).as_posix()
        groups = {"runtime": Counter(), "typecheck": Counter()}
        for case in file["assertionResults"]:
            kind, name = "runtime", case["fullName"]
            if case.get("meta", {}).get("typecheck") is True:
                if file["name"] in runtime_files:
                    continue
                kind = "typecheck"
                ancestors, title = case["ancestorTitles"], case["title"]
                if (not isinstance(ancestors, list) or any(not isinstance(part, str) for part in ancestors)
                        or not isinstance(title, str) or " ".join([*ancestors, title]) != name):
                    raise ValueError("native typecheck identity cannot be verified")
                if ancestors and (package == ancestors[0] or package.endswith("/" + ancestors[0])):
                    ancestors = ancestors[1:]
                name = " ".join([*ancestors, title])
            groups[kind][name] += 1
        for kind, cases in groups.items():
            if cases:
                files[package][kind].append(cases)
    result = Counter()
    for package, groups in files.items():
        # Typechecking an already collected source file and rerunning it in
        # another build mode add execution cost, not new test definitions.
        inventories = groups["runtime"] or groups["typecheck"]
        if any(cases != inventories[0] for cases in inventories[1:]):
            raise ValueError("native mode inventories differ for " + package)
        result.update({(package, name): count for name, count in inventories[0].items()})
    return result


def native_cases(language, records, directory):
    """Keep duplicate occurrences within a command; reject overlapping commands."""
    status = assess_candidate.test_status(records, directory, language)
    if status["status"] != "pass":
        raise ValueError("native collection did not finish successfully: " + json.dumps(status, ensure_ascii=False))
    total = Counter()
    for record in records:
        current = Counter()
        if language == "TypeScript":
            report = json.loads((directory / record["test_report"]).read_text())
            current = vitest_cases(report, Path(record["test_root"]))
        else:
            reports = (record["test_reports"] if language == "Java" else
                       [{"path": record["test_report"]}])
            for report in reports:
                root = ET.parse(directory / report["path"]).getroot()
                suites = [root] if root.tag == "testsuite" else list(root)
                for suite in suites:
                    for case in suite.findall("testcase"):
                        package = record["cwd"] + ":" + case.get("classname", "")
                        current[(package, case.get("name"))] += 1
        if total.keys() & current.keys():
            raise ValueError("native commands overlap the same test identities")
        total.update(current)
    return [{"package": package, "name": name, "occurrences": count}
            for (package, name), count in sorted(total.items())]


def difference(before, after, static):
    missing = []
    if static.get("test_migrations"):
        missing.append("static test migrations have not been matched to native identities")
    for item in static["unknown"]:
        if (item["metric"] in {"tests", "source_and_tests"}
                and not item["reason"].startswith("变化的用例无法完整枚举: ")):
            missing.append(item["file"] + ": " + item["reason"])
    counters = [Counter({(item["package"], item["name"]): item["occurrences"] for item in cases})
                for cases in (before, after)]
    added, removed = counters[1] - counters[0], counters[0] - counters[1]
    if added and removed:
        missing.append("removed and added native identities require migration verification")
    entries = lambda values: [{"package": package, "name": name, "occurrences": count}
                             for (package, name), count in sorted(values.items())]
    return {"native_added_test_count": sum(added.values()) if not missing else None,
            "added_native_identities": entries(added), "removed_native_identities": entries(removed),
            "unknown": missing}


def evidence_file(root, name, expected):
    if not isinstance(name, str) or Path(name).is_absolute() or ".." in Path(name).parts:
        raise ValueError("unsafe evidence path")
    path = (root / name).resolve(strict=True)
    if not path.is_relative_to(root.resolve()) or dataset.digest(path) != expected:
        raise ValueError("recorded evidence changed: " + name)
    return path


def reassess(original_result, output):
    """Recompute quantities from verified native reports, without running tests."""
    original_result = Path(original_result).resolve(strict=True)
    root = original_result.parent
    original = json.loads(original_result.read_text())
    result = {"task_id": original["task_id"], "source_result": str(original_result),
              "source_result_sha256": dataset.digest(original_result), "script_sha256": dataset.digest(Path(__file__)),
              "base_tree": original["base_tree"], "tree": original["tree"],
              "scope": "Verified native source-case quantities; repeated mode execution remains in original cost evidence",
              "native_added_test_count": None, "unknown": [], "complete": False, "variants": {}}
    try:
        if original.get("error") or not all(original[key] for key in
                ("repositories_unchanged", "task_material_unchanged", "static_result_unchanged")):
            raise ValueError("original collection did not preserve verified inputs")
        evidence_file(root, "task.json", original["task_manifest_sha256"])
        static = json.loads(evidence_file(root, "static-quantities.json", original["static_result_sha256"]).read_text())[original["static_side"]]
        if static["base_tree"] != result["base_tree"] or static["tree"] != result["tree"]:
            raise ValueError("static and native fixed trees differ")
        for name, digest in original["scripts_sha256"].items():
            evidence_file(root / "scripts", name, digest)
        for name in ("base", "target"):
            variant = original["variants"][name]
            if not variant["fixed_inputs_unchanged"]:
                raise ValueError("fixed source inputs changed")
            if not passed(variant["versions"]) or variant["prepare"] and not passed(variant["prepare"]):
                raise ValueError("original environment preparation or version probes failed")
            for phase, records in (("prepare", variant["prepare"]), ("versions", variant["versions"]),
                                   ("existing", variant["commands"])):
                for record in records:
                    for stream in ("stdout", "stderr"):
                        evidence_file(root / name / phase, record[stream], record[stream + "_sha256"])
            cases = native_cases(original["language"], variant["commands"], root / name / "existing")
            status = assess_candidate.test_status(variant["commands"], root / name / "existing", original["language"])
            result["variants"][name] = {"cases": cases, "test_count": sum(case["occurrences"] for case in cases),
                                       "executed_test_events": status["test_result_events"]}
        result.update(difference(result["variants"]["base"]["cases"], result["variants"]["target"]["cases"], static))
        result["complete"] = result["native_added_test_count"] is not None
    except (OSError, ValueError, KeyError, TypeError, AttributeError, ET.ParseError) as error:
        result["unknown"].append(str(error))
    save_json(Path(output), result)
    return result


def repository_state(repository):
    index = Path(dataset.git(repository, "rev-parse", "--git-path", "index").stdout.decode().strip())
    if not index.is_absolute():
        index = repository / index
    return {"index_sha256": dataset.digest(index) if index.is_file() else None,
            "index_mtime_ns": index.stat().st_mtime_ns if index.is_file() else None,
            "head": dataset.git(repository, "rev-parse", "HEAD").stdout.decode().strip(),
            "status": dataset.git(repository, "--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=all").stdout.decode(),
            "objects": dataset.git(repository, "count-objects", "-v").stdout.decode()}


def collect(task_directory, source_repository, target_repository, target_tree,
            static_result, output, runtime, static_side="candidate"):
    task_directory = Path(task_directory).resolve(strict=True)
    source_repository = Path(source_repository).resolve(strict=True)
    target_repository = Path(target_repository).resolve(strict=True)
    static_result = Path(static_result).resolve(strict=True)
    output = Path(output).resolve()
    task = dataset.read_task(task_directory)
    if task["language"] not in {"Python", "TypeScript", "Java"}:
        raise ValueError("native quantity reports currently support Python, TypeScript and Java")
    static = json.loads(static_result.read_text())[static_side]
    if static["base_tree"] != task["base_tree"] or static["tree"] != target_tree:
        raise ValueError("static quantities do not match the fixed base and target trees")
    if output.exists():
        raise ValueError("each native quantity attempt needs a new output directory")
    for source in (task_directory, source_repository, target_repository, static_result.parent):
        if output.is_relative_to(source) or source.is_relative_to(output):
            raise ValueError("native results must be separate from source and reference material")
    timeout = runtime.get("timeout_seconds", 600)
    if type(timeout) is not int or not 1 <= timeout <= 3600:
        raise ValueError("timeout must be between 1 and 3600 seconds")
    # Validate identity before any project command runs, including when the
    # target is a candidate tree in a different repository.
    if dataset.git(target_repository, "cat-file", "-t", target_tree).stdout != b"tree\n":
        raise ValueError("target must be a fixed tree object")
    repositories = {str(path): repository_state(path) for path in {source_repository, target_repository}}
    output.mkdir(parents=True, mode=0o700)
    shutil.copy2(static_result, output / "static-quantities.json")
    shutil.copy2(task_directory / "task.json", output / "task.json")
    scripts = output / "scripts"
    scripts.mkdir()
    for name in ("collect_native_quantities.py", "dataset.py", "assess_candidate.py", "validate_dataset.py",
                 "run_agents.py", "trace_command.py", "rust_results.py", "rust_test_inputs.py"):
        shutil.copy2(Path(__file__).parent / name, scripts / name)
    report = {"task_id": task["id"], "language": task["language"], "platform": platform.platform(),
              "task_manifest_sha256": dataset.digest(task_directory / "task.json"),
              "static_result_sha256": dataset.digest(static_result), "static_side": static_side,
              "base_tree": task["base_tree"], "tree": target_tree, "runtime": runtime,
              "started_unix": time.time(), "variants": {}, "native_added_test_count": None,
              "unknown": [], "complete": False, "hidden_tests_added": False,
              "scope": "Full fixed-tree native test quantities; no correctness restoration or hidden acceptance",
              "scripts_sha256": {p.name: dataset.digest(p) for p in scripts.iterdir()}}
    try:
        for name, repository, tree in (("base", source_repository, task["base_tree"]),
                                       ("target", target_repository, target_tree)):
            directory = output / name
            directory.mkdir()
            workspace = directory / "workspace"
            checkout = (dataset.prepare(task_directory, repository, workspace) if name == "base" else
                        dataset.materialize_tree(repository, tree, workspace))
            save_json(directory / "checkout.json", checkout)
            variant = {"checkout": checkout}
            report["variants"][name] = variant
            environment = {**os.environ, **runtime.get("environment", {}),
                           **task["checks"].get("environment", {})}
            environment = {key: substitute(value, workspace, directory) for key, value in environment.items()}
            if task["language"] == "Python":
                environment["PYTHONDONTWRITEBYTECODE"] = "1"
            before = input_fingerprint(workspace, task)
            prepare = directory / "prepare"
            prepare.mkdir()
            variant["prepare"] = run_commands(task["checks"].get("prepare", []), workspace, prepare, environment, timeout)
            if (variant["prepare"] and not passed(variant["prepare"])) or before != input_fingerprint(workspace, task):
                raise ValueError(name + ": preparation failed or changed fixed input files")
            versions = directory / "versions"
            versions.mkdir()
            variant["versions"] = run_commands(runtime.get("versions", []) + task["checks"].get("versions", []),
                                               workspace, versions, environment, timeout)
            if not passed(variant["versions"]):
                raise ValueError(name + ": tool versions were not recorded successfully")
            tests = directory / "existing"
            tests.mkdir()
            commands = assess_candidate.test_commands(task, directory)["existing"]
            if task["language"] == "Java":
                records = assess_candidate.run_maven_commands(commands, workspace, tests, environment, timeout, before)
            else:
                records = run_commands(commands, workspace, tests, environment, timeout)
                for number, record in enumerate(records, 1):
                    suffix = "junit.xml" if task["language"] == "Python" else "vitest.json"
                    path = tests / f"{number}.{suffix}"
                    record.update(test_report=path.name, test_report_sha256=dataset.digest(path) if path.is_file() else None)
                    if task["language"] == "TypeScript":
                        record["test_root"] = str(workspace)
            variant["commands"] = records
            variant["fixed_inputs_unchanged"] = before == input_fingerprint(workspace, task)
            if not variant["fixed_inputs_unchanged"]:
                raise ValueError(name + ": native tests changed fixed input files")
            variant["cases"] = native_cases(task["language"], records, tests)
            variant["test_count"] = sum(case["occurrences"] for case in variant["cases"])
            print(json.dumps({"task_id": task["id"], "variant": name, "native_cases": variant["test_count"]}), flush=True)
        report.update(difference(report["variants"]["base"]["cases"], report["variants"]["target"]["cases"], static))
        report["complete"] = report["native_added_test_count"] is not None
    except (OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.CalledProcessError, ET.ParseError) as error:
        report["error"] = str(error)
        if isinstance(error, subprocess.CalledProcessError):
            report["error_stderr"] = (error.stderr or b"").decode(errors="replace")
    finally:
        report.update(repositories_unchanged=False, task_material_unchanged=False, static_result_unchanged=False)
        try:
            report["repositories_unchanged"] = repositories == {path: repository_state(Path(path)) for path in repositories}
            report["task_material_unchanged"] = dataset.read_task(task_directory) == task
            report["static_result_unchanged"] = report["static_result_sha256"] == dataset.digest(static_result)
        except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
            report["unknown"].append("could not recheck collection inputs: " + str(error))
        if not all(report[key] for key in ("repositories_unchanged", "task_material_unchanged", "static_result_unchanged")):
            report["complete"], report["native_added_test_count"] = False, None
            report["unknown"].append("quantity inputs or source repository changed during collection")
        report["finished_unix"] = time.time()
        save_json(output / "result.json", report)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("task_directory", "source_repository", "target_repository"):
        parser.add_argument(name, type=Path)
    parser.add_argument("target_tree")
    parser.add_argument("static_result", type=Path)
    parser.add_argument("new_output", type=Path)
    parser.add_argument("--runtime", required=True, type=Path)
    parser.add_argument("--static-side", default="candidate", choices=("candidate", "reference"))
    args = parser.parse_args()
    result = collect(args.task_directory, args.source_repository, args.target_repository, args.target_tree,
                     args.static_result, args.new_output, json.loads(args.runtime.read_text()), args.static_side)
    print(json.dumps({key: result[key] for key in ("task_id", "complete", "native_added_test_count", "unknown")}, ensure_ascii=False))
    raise SystemExit(0 if result["complete"] else 1)


if __name__ == "__main__":
    main()
