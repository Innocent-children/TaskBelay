"""Run existing and hidden tests separately on a task's base and reference."""

import argparse
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import time

import dataset
from run_agents import execute, save_json


def run_commands(commands, workspace, output, environment, timeout):
    results = []
    for number, specification in enumerate(commands, 1):
        if isinstance(specification, list):
            argv, relative_cwd = specification, "."
        else:
            argv, relative_cwd = specification["argv"], specification.get("cwd", ".")
        argv = [substitute(arg, workspace, output.parent) for arg in argv]
        cwd = (workspace / relative_cwd).resolve(strict=True)
        if not cwd.is_relative_to(workspace) or not argv or any(not isinstance(arg, str) for arg in argv):
            raise ValueError("invalid evaluation command or working directory")
        stdout = output / f"{number}.stdout.log"
        stderr = output / f"{number}.stderr.log"
        code, timed_out, elapsed = execute(argv, "", cwd, stdout, stderr, timeout, environment)
        result = {"argv": argv, "cwd": relative_cwd, "exit_code": code,
                  "timed_out": timed_out, "elapsed_seconds": elapsed,
                  "stdout": stdout.name, "stderr": stderr.name,
                  "stdout_sha256": dataset.digest(stdout), "stderr_sha256": dataset.digest(stderr)}
        save_json(output / f"{number}.json", result)
        results.append(result)
    return results


def passed(results):
    return bool(results) and all(item["exit_code"] == 0 and not item["timed_out"] for item in results)


def substitute(text, workspace, variant_directory):
    return (text.replace("{workspace}", str(workspace))
            .replace("{variant_directory}", str(variant_directory)).replace("{pathsep}", os.pathsep))


def input_fingerprint(workspace, task):
    paths = {name.decode("utf-8") for name in dataset.git(workspace, "ls-files", "-z").stdout.split(b"\0") if name}
    paths.update(item["path"] for item in task["files"] if item["category"] != "test")
    result = {}
    for name in sorted(paths):
        path = workspace / name
        result[name] = ("symlink:" + os.readlink(path) if path.is_symlink()
                        else dataset.digest(path) if path.is_file() else None)
    return result


def baseline_outcome(results, directory, checks):
    if passed(results):
        return "pass"
    patterns = checks.get("baseline_failure_patterns", [])
    if not results or not patterns or any(item["timed_out"] for item in results):
        return "unknown"
    allowed = checks.get("baseline_failure_exit_codes", [1])
    if any(item["exit_code"] not in [0, *allowed] for item in results):
        return "unknown"
    text = "\n".join((directory / item[field]).read_text(encoding="utf-8", errors="replace")
                     for item in results for field in ("stdout", "stderr"))
    return "fail" if all(re.search(pattern, text) for pattern in patterns) else "unknown"


def validate(task_directory, source_repository, output, runtime):
    task_directory = Path(task_directory).resolve(strict=True)
    source_repository = Path(source_repository).resolve(strict=True)
    output = Path(output).resolve()
    task = dataset.read_task(task_directory)
    checks = task["checks"]
    if not checks.get("existing") or not checks.get("hidden"):
        raise ValueError("native test commands have not been prepared for " + task["id"])
    if output.exists():
        raise ValueError("each validation attempt needs a new output directory")
    for source in (task_directory, source_repository):
        if output.is_relative_to(source) or source.is_relative_to(output):
            raise ValueError("results must be separate from task artifacts and source repository")
    output.mkdir(parents=True, mode=0o700)
    material = output / "task-material"
    material.mkdir()
    for name in ("task.json", *task["sha256"]):
        shutil.copy2(task_directory / name, material / name)
    scripts = output / "scripts"
    scripts.mkdir()
    for name in ("dataset.py", "validate_dataset.py", "run_agents.py", "trace_command.py"):
        shutil.copy2(Path(__file__).parent / name, scripts / name)
    environment = {**os.environ, **runtime.get("environment", {}), **checks.get("environment", {})}
    timeout = runtime.get("timeout_seconds", 600)
    if not 1 <= timeout <= 3600:
        raise ValueError("timeout must be between 1 and 3600 seconds")
    report = {"task_id": task["id"], "task_manifest_sha256": dataset.digest(task_directory / "task.json"),
              "platform": platform.platform(), "started_unix": time.time(),
              "runtime": runtime, "task_environment": checks.get("environment", {}),
              "scripts_sha256": {name: dataset.digest(Path(__file__).parent / name)
                                 for name in ("dataset.py", "validate_dataset.py", "run_agents.py", "trace_command.py")},
              "variants": {}, "validated": False}
    try:
        versions = output / "versions"
        versions.mkdir()
        report["versions"] = run_commands(runtime.get("versions", []), source_repository,
            versions, {key: substitute(value, source_repository, output) for key, value in environment.items()}, timeout)
        for variant in ("base", "reference"):
            root = output / variant
            root.mkdir()
            workspace = root / "workspace"
            receipt = dataset.prepare(task_directory, source_repository, workspace)
            save_json(root / "checkout.json", receipt)
            result = {"checkout": receipt}
            report["variants"][variant] = result
            variant_environment = {key: substitute(value, workspace, root) for key, value in environment.items()}
            if variant == "reference":
                dataset.apply_material(task_directory, workspace, "implementation.patch")
            before_prepare = input_fingerprint(workspace, task)
            for phase in ("prepare", "existing", "hidden"):
                directory = root / phase
                directory.mkdir()
                if phase == "hidden":
                    dataset.apply_material(task_directory, workspace, "hidden.patch")
                commands = checks.get(phase, [])
                result[phase] = run_commands(commands, workspace, directory, variant_environment, timeout)
                print(json.dumps({"task": task["id"], "variant": variant, "phase": phase,
                                  "exit_codes": [item["exit_code"] for item in result[phase]]}), flush=True)
                if phase == "prepare" and commands and not passed(result[phase]):
                    result["preparation_failed"] = True
                    break
                if phase == "prepare":
                    versions = root / "versions"
                    versions.mkdir()
                    result["versions"] = run_commands(
                        runtime.get("versions", []) + checks.get("versions", []),
                        workspace, versions, variant_environment, timeout)
                    result["lockfiles"] = {str(path.relative_to(workspace)): dataset.digest(path)
                        for name in ("go.sum", "Cargo.lock", "pnpm-lock.yaml", "package-lock.json", "uv.lock")
                        if (path := workspace / name).is_file()}
                    after_prepare = input_fingerprint(workspace, task)
                    result["preparation_changed_inputs"] = [name for name in before_prepare
                        if before_prepare[name] != after_prepare[name]]
                    if result["preparation_changed_inputs"]:
                        result["preparation_failed"] = True
                        break
        base, reference = report["variants"]["base"], report["variants"]["reference"]
        baseline_expected = checks.get("baseline_hidden", "fail")
        baseline_observed = baseline_outcome(base.get("hidden", []), output / "base" / "hidden", checks)
        report["baseline_hidden_outcome"] = baseline_observed
        report["baseline_hidden_expectation_met"] = bool(base.get("hidden")) and baseline_expected == baseline_observed
        report["validated"] = (passed(base.get("existing", [])) and passed(reference.get("existing", []))
            and passed(reference.get("hidden", [])) and report["baseline_hidden_expectation_met"]
            and not base.get("preparation_failed") and not reference.get("preparation_failed")
            and passed(report["versions"]) and passed(base.get("versions", []))
            and passed(reference.get("versions", [])))
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        report["error"] = str(error)
        if isinstance(error, subprocess.CalledProcessError):
            report["error_stderr"] = (error.stderr or b"").decode("utf-8", errors="replace")
    finally:
        report["finished_unix"] = time.time()
        save_json(output / "result.json", report)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("task_directory", type=Path)
    parser.add_argument("source_repository", type=Path)
    parser.add_argument("new_output", type=Path)
    parser.add_argument("--runtime", type=Path, required=True)
    arguments = parser.parse_args()
    runtime = json.loads(arguments.runtime.read_text(encoding="utf-8"))
    result = validate(arguments.task_directory, arguments.source_repository, arguments.new_output, runtime)
    print(json.dumps({"task": result["task_id"], "validated": result["validated"], "error": result.get("error")}))
    raise SystemExit(0 if result["validated"] else 1)


if __name__ == "__main__":
    main()
