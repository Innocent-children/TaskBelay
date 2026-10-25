"""Check completed Agent candidates against unchanged existing and hidden tests."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import subprocess
import time
import xml.etree.ElementTree as ET

import dataset
import rust_results
import rust_test_inputs
from run_agents import save_json
from validate_dataset import input_fingerprint, passed, run_commands, substitute

TEST_RESULT_FORMATS = {"Go": "go-json", "Python": "pytest-junit", "TypeScript": "vitest-json",
                       "Java": "surefire-junit", "Rust": "cargo-json-libtest-pretty"}


def working_files(workspace):
    names = dataset.git(workspace, "ls-files", "--cached", "--others", "--exclude-standard", "-z").stdout
    files = {}
    for name in sorted(set(names.split(b"\0")) - {b""}):
        relative = PurePosixPath(name.decode("utf-8"))
        if relative.is_absolute() or any(part in {".", ".."} or part.lower() == ".git" for part in relative.parts):
            raise ValueError("unsafe candidate path")
        path = workspace / relative
        if not path.exists() and not path.is_symlink():
            continue
        if path.is_symlink():
            target = os.readlink(path)
            if Path(target).is_absolute() or not path.resolve().is_relative_to(workspace):
                raise ValueError("candidate symlink leaves workspace: " + str(relative))
            files[str(relative)] = ("120000", target.encode("utf-8"))
        elif path.is_file() and path.resolve().is_relative_to(workspace):
            files[str(relative)] = ("100755" if path.stat().st_mode & 0o111 else "100644", path.read_bytes())
        else:
            raise ValueError("unsupported candidate path: " + str(relative))
    return files


def put_file(root, name, mode, data):
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.is_symlink() or path.is_file():
        path.unlink()
    elif path.is_dir():
        path.rmdir()
    if mode == "120000":
        path.symlink_to(data.decode("utf-8"))
    else:
        path.write_bytes(data)
        path.chmod(0o755 if mode == "100755" else 0o644)


def go_test_file(name):
    return name.endswith("_test.go") or "testdata" in PurePosixPath(name).parts


def go_test_status(results, directory):
    events, invalid = [], []
    for result in results:
        for number, line in enumerate((directory / result["stdout"]).read_text(errors="replace").splitlines(), 1):
            try:
                event = json.loads(line)
                if not isinstance(event, dict):
                    raise ValueError("non-object event")
                events.append(event)
            except ValueError:
                invalid.append({"file": result["stdout"], "line": number})
    tests = [event for event in events if event.get("Test")]
    failures = [event for event in events if event.get("Action") == "fail"]
    executed = [event for event in tests if event.get("Action") in {"pass", "fail"}]
    status = "unknown"
    if results and not any(item["timed_out"] for item in results) and not invalid:
        if passed(results) and executed and not failures:
            status = "pass"
        elif failures and any(item["exit_code"] != 0 for item in results):
            status = "fail"
    return {"status": status, "test_result_events": len(executed),
            "skipped": [event for event in tests if event.get("Action") == "skip"],
            "failures": failures, "invalid_json_lines": invalid}


def python_test_file(name):
    path = PurePosixPath(name)
    return (any(part in {"test", "tests"} for part in path.parts[:-1]) or path.name == "conftest.py"
            or path.name.startswith("test_") and path.suffix == ".py" or path.name.endswith("_test.py"))


def typescript_test_file(name):
    path = PurePosixPath(name)
    # The fixed Vitest typecheck commands also collect type assertion files.
    return (".test." in path.name or ".spec." in path.name or "__tests__" in path.parts[:-1]
            or path.name.endswith((".test-d.ts", ".test-d.tsx")))


def typescript_configuration_file(name):
    name = PurePosixPath(name).name
    return (name.startswith(("vitest.", "vite.config.", "tsconfig"))
            or name in {"package.json", "package-lock.json", "pnpm-lock.yaml", "pnpm-workspace.yaml",
                        "yarn.lock", "bun.lock", "bun.lockb", "nub.lock", ".npmrc"})


def java_test_file(name):
    return "/src/test/" in "/" + name


def java_configuration_file(name):
    path = PurePosixPath(name)
    return path.name in {"pom.xml", "mvnw", "mvnw.cmd"} or ".mvn" in path.parts


def run_maven_commands(commands, workspace, output, environment, timeout, source_files):
    results = []
    for number, command in enumerate(commands, 1):
        directory = output / str(number)
        directory.mkdir()
        cwd = command.get("cwd", ".") if isinstance(command, dict) else "."
        reports = workspace / cwd / "target/surefire-reports"
        if reports.resolve() != reports.absolute() or not reports.resolve().is_relative_to(workspace):
            raise ValueError("native Maven report directory must stay inside the validation copy without symlinks")
        prefix = reports.relative_to(workspace).as_posix() + "/"
        if any(name.startswith(prefix) or name == prefix[:-1] for name in source_files):
            raise ValueError("native Maven reports overlap candidate source inputs")
        if reports.exists():
            shutil.move(str(reports), str(directory / "previous-reports"))
        result = run_commands([command], workspace, directory, environment, timeout)[0]
        for stream in ("stdout", "stderr"):
            result[stream] = str(number) + "/" + result[stream]
        saved = directory / "reports"
        if reports.exists():
            if reports.resolve() != reports.absolute() or any(path.is_symlink() for path in reports.rglob("*")):
                raise ValueError("native Maven reports contain symlinks")
            shutil.copytree(reports, saved)
        result["test_reports"] = [{"path": path.relative_to(output).as_posix(), "sha256": dataset.digest(path)}
                                  for path in sorted(saved.glob("TEST-*.xml"))]
        result["test_report_directory"] = saved.relative_to(output).as_posix()
        result["test_report_diagnostics"] = [{"path": path.relative_to(output).as_posix(), "sha256": dataset.digest(path)}
            for path in sorted(saved.glob("*")) if path.is_file() and path.suffix in {".dump", ".dumpstream"}]
        results.append(result)
    return results


def vitest_status(results, directory):
    failures, skipped, invalid, outcomes = [], [], [], []
    executed = 0
    for result in results:
        name = result.get("test_report")
        try:
            if not name or Path(name).name != name:
                raise ValueError("missing or unsafe native report path")
            path = (directory / name).resolve(strict=True)
            if not path.is_relative_to(directory.resolve()) or dataset.digest(path) != result.get("test_report_sha256"):
                raise ValueError("native report is outside the evidence directory or changed")
            report = json.loads(path.read_text())
            root = Path(result["test_root"])
            if not root.is_absolute() or not isinstance(report["testResults"], list):
                raise ValueError("native test root or file results are missing")
            counts = {key: 0 for key in ("passed", "failed", "skipped", "pending", "todo")}
            failed_files = 0
            for file in report["testResults"]:
                file_path = Path(file["name"])
                if not file_path.is_absolute() or ".." in file_path.parts:
                    raise ValueError("native test file is outside the validation root")
                package = str(file_path.relative_to(root))
                if file["status"] not in {"passed", "failed"} or not isinstance(file["assertionResults"], list):
                    raise ValueError("native test file has an unknown outcome")
                failed_cases = 0
                for case_index, case in enumerate(file["assertionResults"]):
                    status, title = case["status"], case["fullName"]
                    if status not in counts or not isinstance(title, str):
                        raise ValueError("native test case has no identity or terminal outcome")
                    counts[status] += 1
                    evidence = {"Package": package, "Test": title, "case_index": case_index, "kind": status}
                    if status == "failed":
                        messages = case["failureMessages"]
                        if not isinstance(messages, list) or any(not isinstance(message, str) for message in messages):
                            raise ValueError("native failure details are malformed")
                        failures.append({**evidence, "detail": "\n".join(messages),
                                         "typecheck": case.get("meta", {}).get("typecheck", False)})
                        failed_cases += 1
                    elif status in {"skipped", "todo"}:
                        skipped.append(evidence)
                if file["status"] == "failed":
                    failed_files += 1
                    if not failed_cases:
                        failures.append({"Package": package, "Test": "", "kind": "file",
                                         "detail": file.get("message", "")})
                elif failed_cases:
                    raise ValueError("native file passed despite failed test cases")
            expected = {"numTotalTests": sum(counts.values()), "numPassedTests": counts["passed"],
                        "numFailedTests": counts["failed"], "numPendingTests": counts["skipped"] + counts["pending"],
                        "numTodoTests": counts["todo"]}
            if any(type(report.get(key)) is not int or report[key] != value for key, value in expected.items()):
                raise ValueError("native totals differ from test case evidence")
            suite_keys = ("numTotalTestSuites", "numPassedTestSuites", "numFailedTestSuites", "numPendingTestSuites")
            if (any(type(report.get(key)) is not int or report[key] < 0 for key in suite_keys)
                    or report[suite_keys[0]] != sum(report[key] for key in suite_keys[1:])
                    or report["numTotalTestSuites"] < len(report["testResults"])
                    or report["numFailedTestSuites"] < failed_files):
                raise ValueError("native suite totals are inconsistent")
            failed = bool(failed_files or counts["failed"] or report["numFailedTestSuites"])
            if type(report.get("success")) is not bool or report["success"] == failed:
                raise ValueError("native success flag contradicts recorded failures")
            count = counts["passed"] + counts["failed"]
            executed += count
            status = "unknown"
            if not result["timed_out"] and not counts["pending"]:
                if result["exit_code"] == 0 and count and not failed:
                    status = "pass"
                elif result["exit_code"] == 1 and failed:
                    status = "fail"
            outcomes.append(status)
        except (OSError, ValueError, TypeError, KeyError, AttributeError) as error:
            invalid.append({"file": name, "error": str(error)})
            outcomes.append("unknown")
    status = "unknown"
    if outcomes and "unknown" not in outcomes:
        status = "fail" if "fail" in outcomes else "pass"
    return {"status": status, "test_result_events": executed, "skipped": skipped,
            "failures": failures, "invalid_reports": invalid}


def junit_status(results, directory, maven=False):
    failures, skipped, invalid, outcomes = [], [], [], []
    executed = 0

    def native_path(name, expected):
        if (not isinstance(name, str) or not name or Path(name).is_absolute()
                or ".." in PurePosixPath(name).parts):
            raise ValueError("missing or unsafe native report path")
        path = (directory / name).resolve(strict=True)
        if not path.is_relative_to(directory.resolve()) or dataset.digest(path) != expected:
            raise ValueError("native report is outside the evidence directory or changed")
        return path

    for result in results:
        name = result.get("test_report_directory") if maven else result.get("test_report")
        try:
            if not maven and (not name or Path(name).name != name):
                raise ValueError("missing or unsafe native report path")
            reports = result.get("test_reports") if maven else [
                {"path": name, "sha256": result.get("test_report_sha256")}]
            if not isinstance(reports, list) or not reports or len({item["path"] for item in reports}) != len(reports):
                raise ValueError("native JUnit reports are missing or duplicated")
            if maven:
                folder = (directory / name).resolve(strict=True)
                if not folder.is_relative_to(directory.resolve()):
                    raise ValueError("native report directory is outside the evidence directory")
                actual = {path.relative_to(directory.resolve()).as_posix() for path in folder.glob("TEST-*.xml")}
                if actual != {item["path"] for item in reports}:
                    raise ValueError("native JUnit report inventory changed")
                if result.get("test_report_diagnostics") or any(folder.glob("*.dump*")):
                    raise ValueError("native Surefire diagnostics indicate an incomplete test process")
            count, failed = 0, False
            aggregate = {key: 0 for key in ("tests", "failures", "errors", "skipped")}
            for report in reports:
                root = ET.parse(native_path(report["path"], report["sha256"])).getroot()
                suites = [root] if root.tag == "testsuite" else list(root) if root.tag == "testsuites" else []
                if not suites or any(suite.tag != "testsuite" for suite in suites):
                    raise ValueError("expected native JUnit test suites")
                for suite in suites:
                    cases = suite.findall("testcase")
                    totals = {key: 0 for key in aggregate}
                    if maven and int(suite.get("flakes", "0")):
                        raise ValueError("native Surefire reruns cannot replace the original failure")
                    for case in cases:
                        if not case.get("name") or maven and not case.get("classname"):
                            raise ValueError("test case has no identity")
                        if maven and any(child.tag in {"flakyFailure", "flakyError", "rerunFailure", "rerunError"} for child in case):
                            raise ValueError("native Surefire reruns cannot replace the original failure")
                        terminals = [child for child in case if child.tag in {"failure", "error", "skipped"}]
                        if len(terminals) > 1:
                            raise ValueError("test case has contradictory terminal outcomes")
                        totals["tests"] += 1
                        if not terminals:
                            count += 1
                            continue
                        terminal = terminals[0]
                        key = {"failure": "failures", "error": "errors", "skipped": "skipped"}[terminal.tag]
                        totals[key] += 1
                        evidence = {"Package": case.get("classname", ""), "Test": case.get("name"),
                                    "kind": terminal.tag, "message": terminal.get("message", ""),
                                    "detail": terminal.text or ""}
                        if terminal.tag == "skipped":
                            skipped.append(evidence)
                        else:
                            failures.append(evidence)
                            failed = True
                            count += 1
                    if any(int(suite.get(key, "-1")) != value for key, value in totals.items()):
                        raise ValueError("native suite totals differ from test case evidence")
                    for key, value in totals.items():
                        aggregate[key] += value
            if maven:
                streams = [native_path(result[stream], result[stream + "_sha256"]).read_text(errors="replace")
                           for stream in ("stdout", "stderr")]
                text = re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", "\n".join(streams))
                summaries = re.findall(r"(?m)^\[(?:INFO|ERROR|WARNING)\] Tests run: (\d+), Failures: (\d+), Errors: (\d+), Skipped: (\d+)\s*$", text)
                if len(summaries) != 1 or tuple(map(int, summaries[0])) != tuple(aggregate.values()):
                    raise ValueError("native Maven summary differs from complete XML evidence")
                expected_build = "FAILURE" if failed else "SUCCESS"
                if re.findall(r"(?m)^\[(?:INFO|ERROR)\] BUILD (SUCCESS|FAILURE)\s*$", text) != [expected_build]:
                    raise ValueError("native Maven build outcome is incomplete or contradicts tests")
            executed += count
            status = "unknown"
            if not result["timed_out"]:
                if result["exit_code"] == 0 and count and not failed:
                    status = "pass"
                elif result["exit_code"] == 1 and failed:
                    status = "fail"
            outcomes.append(status)
        except (OSError, ValueError, KeyError, TypeError, AttributeError, ET.ParseError) as error:
            invalid.append({"file": name, "error": str(error)})
            outcomes.append("unknown")
    status = "unknown"
    if outcomes and "unknown" not in outcomes:
        status = "fail" if "fail" in outcomes else "pass"
    return {"status": status, "test_result_events": executed, "skipped": skipped,
            "failures": failures, "invalid_reports": invalid}


def pytest_status(results, directory):
    return junit_status(results, directory)


def rust_context(report, assessment, task, phase, existing_status=None):
    directory = assessment / "rust-test-inputs"
    for name, key in (("input.json", "input_sha256"), ("stdout.json", "output_sha256"), ("result.json", "result_sha256")):
        if dataset.digest(directory / name) != report["rust_protection"][key]:
            raise ValueError("Rust test protection evidence changed: " + name)
    protection = json.loads((directory / "result.json").read_text())
    if any(protection[key] != report["rust_protection"][key] for key in ("input_sha256", "output_sha256")):
        raise ValueError("Rust protection receipt does not match its native inputs")
    # Cargo keeps native absolute paths even after the evidence directory moves.
    test_root = report.get("rust_test_root", str(assessment / "validation"))
    if not isinstance(test_root, str) or not Path(test_root).is_absolute():
        raise ValueError("Rust native test root must be an absolute path")
    context = {"test_root": test_root, "additional_doc_ranges": protection["additional_doc_ranges"]}
    if phase == "hidden":
        hidden = assessment / "rust-hidden-inputs.json"
        if dataset.digest(hidden) != report["rust_hidden_inputs_sha256"]:
            raise ValueError("Rust hidden input evidence changed")
        context.update(hidden_inputs=json.loads(hidden.read_text()),
                       baseline_failure_patterns=task["checks"].get("baseline_failure_patterns", []),
                       existing_passed=(existing_status if existing_status is not None else report["existing"]["status"]) == "pass")
    return context


def test_status(results, directory, language="Go", context=None):
    if language == "Go":
        return go_test_status(results, directory)
    if language == "Python":
        return pytest_status(results, directory)
    if language == "TypeScript":
        return vitest_status(results, directory)
    if language == "Java":
        return junit_status(results, directory, maven=True)
    if language == "Rust":
        return rust_results.rust_status(results, directory, context)
    raise ValueError("unsupported candidate result language: " + str(language))


def test_commands(task, output):
    commands = {}
    for phase in ("existing", "hidden"):
        commands[phase] = []
        for number, specification in enumerate(task["checks"].get(phase, []), 1):
            argv = specification.get("argv") if isinstance(specification, dict) else specification
            if not isinstance(argv, list) or not all(isinstance(arg, str) for arg in argv):
                raise ValueError("native test commands must be argument lists")
            if task["language"] == "Go":
                if argv[:2] != ["go", "test"]:
                    raise ValueError("Go candidates require native go test commands")
                command = [*argv[:2], "-count=1", "-json", *argv[2:]]
            elif task["language"] == "Python":
                if (len(argv) < 3 or argv[1:3] != ["-m", "pytest"]
                        or not re.fullmatch(r"python(?:3(?:\.\d+)?)?(?:\.exe)?", Path(argv[0]).name)
                        or any(arg.startswith(("--junitxml", "--junit-xml")) for arg in argv)):
                    raise ValueError("Python candidates require native python -m pytest commands")
                command = [*argv, "--junitxml=" + str(output / phase / f"{number}.junit.xml"),
                           "-o", "junit_family=xunit2"]
            elif task["language"] == "TypeScript":
                prefix = (1 if argv and Path(argv[0]).name in {"vitest", "vitest.cmd", "vitest.mjs"} else
                          2 if len(argv) > 1 and Path(argv[0]).name in {"node", "node.exe"} and Path(argv[1]).name == "vitest.mjs" else
                          3 if argv[:3] == ["pnpm", "exec", "vitest"] else
                          4 if argv[:4] == ["nub", "exec", "--node", "vitest"] else 0)
                if (not prefix or argv[prefix:prefix+1] != ["run"]
                        or any(arg.startswith(("--reporter", "--outputFile")) for arg in argv[prefix:])):
                    raise ValueError("TypeScript candidates require native Vitest run commands")
                command = [*argv, "--reporter=default", "--reporter=json",
                           "--outputFile=" + str(output / phase / f"{number}.vitest.json")]
            elif task["language"] == "Java":
                if not argv or Path(argv[0]).name not in {"mvn", "mvn.cmd", "mvnw", "mvnw.cmd"} or "test" not in argv:
                    raise ValueError("Java candidates require native Maven test commands")
                command = argv
            elif task["language"] == "Rust":
                start = 2 if len(argv) > 1 and argv[1].startswith("+") else 1
                if (not argv or Path(argv[0]).name not in {"cargo", "cargo.exe"} or argv[start:start+1] != ["test"]
                        or any(arg.startswith("--message-format") or arg in {"--", "--no-run", "--list", "-q", "--quiet"}
                               for arg in argv[start+1:])):
                    raise ValueError("Rust candidates require native Cargo test commands without conflicting reporters")
                command = [*argv, "--message-format=json", "--", "--format=pretty", "--color=never"]
            else:
                raise ValueError("unsupported candidate language: " + str(task["language"]))
            commands[phase].append({**specification, "argv": command} if isinstance(specification, dict) else command)
        if not commands[phase]:
            raise ValueError("both existing and hidden tests are required")
    return commands


def assess(task_directory, source_repository, workspace, agent_directory, output, runtime, required_language=None):
    task_directory, source_repository, workspace, agent_directory = (
        Path(path).resolve(strict=True) for path in (task_directory, source_repository, workspace, agent_directory))
    output = Path(output).resolve()
    task = dataset.read_task(task_directory)
    language = task.get("language")
    if language not in TEST_RESULT_FORMATS or required_language and language != required_language:
        raise ValueError("unsupported candidate language: " + str(language))
    supports = runtime.get("test_support_files", []) if language == "TypeScript" else []
    if (not isinstance(supports, list) or any(not isinstance(name, str) or not name
            or PurePosixPath(name).is_absolute() or any(part in {".", "..", ".git"} for part in PurePosixPath(name).parts)
            for name in supports)):
        raise ValueError("test support files must be workspace-relative paths")
    classifier = {"Go": go_test_file, "Python": python_test_file,
                  "TypeScript": typescript_test_file, "Java": java_test_file, "Rust": rust_test_inputs.test_file}[language]
    is_test = lambda name: classifier(name) or name in supports
    for source in (task_directory, source_repository, workspace, agent_directory):
        if output.is_relative_to(source) or source.is_relative_to(output):
            raise ValueError("assessment must be outside all source and Agent directories")
    agent = json.loads((agent_directory / "result.json").read_text())
    metadata = json.loads((agent_directory / "metadata.json").read_text())
    if Path(metadata["workspace"]).resolve() != workspace or metadata["task_id"] != task["id"]:
        raise ValueError("Agent receipt does not match the task and workspace")
    if not agent.get("turns") or any("returncode" not in turn for turn in agent["turns"]):
        raise ValueError("a completed Agent process receipt is required")
    timeout = runtime.get("timeout_seconds", 600)
    if not 1 <= timeout <= 3600:
        raise ValueError("timeout must be between 1 and 3600 seconds")
    commands = test_commands(task, output)
    files = working_files(workspace)
    if files != working_files(workspace):
        raise ValueError("candidate changed during collection")
    output.mkdir(parents=True, mode=0o700, exist_ok=False)
    material = output / "task-material"
    material.mkdir()
    for name in ("task.json", *task["sha256"]):
        shutil.copy2(task_directory / name, material / name)
    scripts = output / "scripts"
    scripts.mkdir()
    for name in ("assess_candidate.py", "assess_go_candidate.py", "dataset.py", "validate_dataset.py", "run_agents.py",
                 "trace_command.py", "rust_results.py", "rust_test_inputs.py"):
        shutil.copy2(Path(__file__).parent / name, scripts / name)
    report = {"task_id": task["id"], "language": language,
              "test_result_format": TEST_RESULT_FORMATS[language],
              "platform": platform.platform(), "runtime": runtime,
              "started_unix": time.time(), "agent_evidence": str(agent_directory),
              "agent_result_sha256": dataset.digest(agent_directory / "result.json"),
              "agent_metadata_sha256": dataset.digest(agent_directory / "metadata.json"),
              "valid_integration": agent["valid_integration"], "existing": {"status": "unknown"},
              "hidden": {"status": "unknown"}, "correctness_evaluated": False,
              "metrics_evaluated": False, "scripts_sha256": {
                  path.name: dataset.digest(path) for path in scripts.iterdir()}}
    try:
        candidate = output / "candidate"
        dataset.prepare(material, source_repository, candidate)
        base_files = dataset.tree_entries(candidate, task["base_tree"])
        for name, _, _ in base_files:
            (candidate / name).unlink()
        for name, (mode, data) in files.items():
            put_file(candidate, name, mode, data)
        # This index belongs to the new evidence copy. Every selected file is read
        # afresh, including originally tracked files now matched by ignore rules.
        dataset.git(candidate, "read-tree", "--empty")
        dataset.git(candidate, "add", "--all", "--force", "--", ".")
        tree = dataset.git(candidate, "write-tree").stdout.decode().strip()
        patch = output / "candidate.patch"
        patch.write_bytes(dataset.git(candidate, "diff", "--no-ext-diff", "--no-textconv", "--binary", task["base_tree"], tree).stdout)
        report.update(candidate_tree=tree, candidate_patch_sha256=dataset.digest(patch),
                      candidate_files={name: {"mode": mode, "sha256": hashlib.sha256(data).hexdigest()}
                                       for name, (mode, data) in files.items()})
        if language in {"Python", "TypeScript", "Java", "Rust"}:
            config_names = {"pytest.ini", ".pytest.ini", "pytest.toml", ".pytest.toml",
                            "pyproject.toml", "setup.cfg", "tox.ini"}
            is_config = {"Python": lambda name: PurePosixPath(name).name in config_names,
                         "TypeScript": typescript_configuration_file, "Java": java_configuration_file,
                         "Rust": rust_test_inputs.configuration_file}[language]
            original_configs = {str(name): (mode, dataset.git(candidate, "cat-file", "blob", blob).stdout)
                                for name, mode, blob in base_files if is_config(str(name))}
            candidate_configs = {name: value for name, value in files.items() if is_config(name)}
            if original_configs != candidate_configs:
                report["changed_test_configuration"] = sorted(name for name in original_configs.keys() | candidate_configs.keys()
                    if original_configs.get(name) != candidate_configs.get(name))
                raise ValueError(language + " test collection configuration changed; original coverage is unverified")
            if language == "TypeScript":
                if any(name not in {str(path) for path, _, _ in base_files} for name in supports):
                    raise ValueError("test support file is missing from the original task")
                if not supports and any(re.search(rb"\b(?:setupFiles|globalSetup)\s*:", data)
                                        for name, (_, data) in original_configs.items() if PurePosixPath(name).name.startswith("vitest.")):
                    raise ValueError("Vitest setup inputs must be declared in test_support_files")
        validation = output / "validation"
        shutil.copytree(candidate, validation, symlinks=True)
        originals = {}
        if language == "Rust":
            report["rust_test_root"] = str(validation)
            protected = rust_test_inputs.protect(files, rust_test_inputs.original_files(candidate, base_files), validation,
                output / "rust-test-inputs", runtime["rust_test_inputs"], put_file)
            report["rust_protection"] = {**protected, "result_sha256": dataset.digest(output / "rust-test-inputs/result.json")}
            originals = {str(name): {"blob": blob, "mode": mode} for name, mode, blob in base_files
                         if str(name) in protected["original_test_files"]}
        else:
            for name in files:
                if is_test(name):
                    (validation / name).unlink()
            for name, mode, blob in base_files:
                if is_test(str(name)):
                    data = dataset.git(candidate, "cat-file", "blob", blob).stdout
                    put_file(validation, str(name), mode, data)
                    originals[str(name)] = {"blob": blob, "mode": mode}
        report["original_test_files"] = originals
        environment = {**os.environ, **runtime.get("environment", {}), **task["checks"].get("environment", {})}
        environment = {key: substitute(value, validation, output) for key, value in environment.items()}
        if language == "Python":
            environment["PYTHONDONTWRITEBYTECODE"] = "1"
        if language == "Rust":
            environment["CARGO_TERM_COLOR"] = "never"
        before = input_fingerprint(validation, task)
        prepare = output / "prepare"
        prepare.mkdir()
        report["prepare"] = run_commands(task["checks"].get("prepare", []), validation, prepare, environment, timeout)
        after = input_fingerprint(validation, task)
        if (report["prepare"] and not passed(report["prepare"])) or before != after:
            raise ValueError("validation preparation failed or changed source inputs")
        versions = output / "versions"
        versions.mkdir()
        report["versions"] = run_commands(runtime.get("versions", []) + task["checks"].get("versions", []),
                                           validation, versions, environment, timeout)
        if not passed(report["versions"]):
            raise ValueError("tool versions were not successfully recorded")
        for phase in ("existing", "hidden"):
            directory = output / phase
            directory.mkdir()
            if phase == "hidden":
                changed = dataset.git(validation, "apply", "--numstat", "-z", str(material / "hidden.patch")).stdout
                names = [item.split(b"\t", 2)[2].decode("utf-8") for item in changed.split(b"\0") if item]
                if not names or any(not is_test(name) for name in names):
                    raise ValueError("hidden acceptance changes non-test files")
                before_hidden = working_files(validation) if language == "Rust" else {}
                dataset.apply_material(material, validation, "hidden.patch")
                if language == "Rust":
                    after_hidden = working_files(validation)
                    hidden_inputs = {name: {key: base64.b64encode(values.get(name, (None, b""))[1]).decode("ascii")
                                           for key, values in (("before", before_hidden), ("after", after_hidden))} for name in names}
                    save_json(output / "rust-hidden-inputs.json", hidden_inputs)
                    report["rust_hidden_inputs_sha256"] = dataset.digest(output / "rust-hidden-inputs.json")
            inputs = working_files(validation)
            if language == "Java":
                source_files = files.keys() | {str(name) for name, _, _ in base_files}
                records = run_maven_commands(commands[phase], validation, directory, environment, timeout, source_files)
            else:
                records = run_commands(commands[phase], validation, directory, environment, timeout)
            if language in {"Python", "TypeScript"}:
                for number, record in enumerate(records, 1):
                    suffix = "junit.xml" if language == "Python" else "vitest.json"
                    path = directory / f"{number}.{suffix}"
                    record["test_report"] = path.name
                    record["test_report_sha256"] = dataset.digest(path) if path.is_file() else None
                    if language == "TypeScript":
                        record["test_root"] = str(validation)
            context = rust_context(report, output, task, phase) if language == "Rust" else None
            report[phase] = {**test_status(records, directory, language, context), "commands": records}
            after = working_files(validation)
            changed = [name for name in inputs if inputs[name] != after.get(name)]
            if changed:
                report[phase]["status"] = "unknown"
                report[phase]["changed_inputs"] = changed
        if files != working_files(workspace):
            raise ValueError("Agent workspace changed after candidate collection")
        report["correctness_evaluated"] = all(report[phase]["status"] != "unknown" for phase in ("existing", "hidden"))
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        report["error"] = str(error)
        if isinstance(error, subprocess.CalledProcessError):
            report["error_stderr"] = (error.stderr or b"").decode("utf-8", errors="replace")
    finally:
        report["finished_unix"] = time.time()
        report["valid_correctness_sample"] = report["valid_integration"] and report["correctness_evaluated"] and "error" not in report
        save_json(output / "result.json", report)
    return report


def main(required_language=None):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("task_directory", "source_repository", "workspace", "agent_directory", "output"):
        parser.add_argument(name, type=Path)
    parser.add_argument("--runtime", type=Path, required=True)
    args = parser.parse_args()
    result = assess(args.task_directory, args.source_repository, args.workspace, args.agent_directory,
                    args.output, json.loads(args.runtime.read_text()), required_language=required_language)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    raise SystemExit(0 if result["valid_correctness_sample"] else 1)


if __name__ == "__main__":
    main()
