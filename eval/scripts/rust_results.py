"""Read stable Cargo build messages and libtest output for evaluation."""

import hashlib
import base64
import difflib
import json
from pathlib import Path, PurePosixPath
import re


START = re.compile(r"running (\d+) tests?")
CASE = re.compile(r"test (.+) \.\.\. (ok|FAILED|ignored)(?:, (.*))?")
SUMMARY = re.compile(r"test result: (ok|FAILED)\. (\d+) passed; (\d+) failed; (\d+) ignored; "
                     r"(\d+) measured; (\d+) filtered out; finished in [\d.]+s")
MERGED = re.compile(r"all doctests ran in [\d.]+s; merged doctests compilation took [\d.]+s")


def harness_suites(lines):
    suites, merged = [], []
    current = None
    failure_output = False
    for line in lines:
        if not line.strip():
            continue
        start, case, summary = START.fullmatch(line), CASE.fullmatch(line), SUMMARY.fullmatch(line)
        if start and not failure_output:
            if current is not None:
                raise ValueError("test suite started before the previous summary")
            current = {"declared": int(start[1]), "cases": []}
        elif summary:
            if current is None:
                raise ValueError("test summary without a started suite")
            counts = [int(summary[index]) for index in range(2, 7)]
            cases = current["cases"]
            actual = [sum(case["status"] == kind for case in cases) for kind in ("ok", "FAILED", "ignored")]
            if (counts[:3] != actual or sum(actual) != current["declared"] or any(counts[3:])
                    or len({case["name"] for case in cases}) != len(cases)
                    or (summary[1] == "FAILED") != bool(counts[1])):
                raise ValueError("test cases and native summary disagree")
            suites.append(current)
            current, failure_output = None, False
        elif case and current is not None and not failure_output:
            if case[3] is not None and case[2] != "ignored":
                raise ValueError("unexpected native case annotation")
            current["cases"].append({"name": case[1], "status": case[2], "reason": case[3]})
        elif line == "failures:" and current is not None:
            failure_output = True
        elif MERGED.fullmatch(line) and current is None and suites:
            if len(suites) in merged:
                raise ValueError("duplicate merged doctest summary")
            merged.append(len(suites))
        elif not failure_output:
            raise ValueError("unrecognized test output: " + line[:160])
    if current is not None:
        raise ValueError("test suite has no terminal summary")
    return suites, merged


def native_path(value):
    return str(PurePosixPath(value.replace("\\", "/")))


def source_path(value, root, base):
    path = Path(value)
    if not path.is_absolute():
        path = Path(base) / path
    path = path.resolve()
    return path.relative_to(Path(root).resolve()).as_posix()


def hidden_ranges(inputs):
    ranges = {}
    for name, values in inputs.items():
        before, after = (base64.b64decode(values[key], validate=True).decode("utf-8").splitlines()
                         for key in ("before", "after"))
        ranges[name] = [(new_start + 1, new_end)
                        for kind, _, _, new_start, new_end in difflib.SequenceMatcher(None, before, after, autojunk=False).get_opcodes()
                        if kind in {"insert", "replace"} and new_end > new_start]
    return ranges


def hidden_compiler_failures(errors, result, context, command_index):
    if (not context or not context.get("existing_passed") or not errors
            or result["timed_out"] or result["exit_code"] != 101):
        return []
    patterns = context.get("baseline_failure_patterns", [])
    rendered = "\n".join(error["message"].get("rendered") or "" for error in errors)
    if not patterns or not all(re.search(pattern, rendered) for pattern in patterns):
        return []
    ranges = hidden_ranges(context.get("hidden_inputs", {}))
    failures = []
    for error in errors:
        if "test" not in error["target"]["kind"]:
            return []
        spans = [span for span in error["message"]["spans"] if span["is_primary"]]
        if not spans:
            return []
        for span in spans:
            name = source_path(span["file_name"], context["test_root"], Path(context["test_root"]) / result.get("cwd", "."))
            if not any(first <= span["line_start"] <= span["line_end"] <= last for first, last in ranges.get(name, [])):
                return []
        code = error["message"].get("code") or {}
        failures.append({"kind": "compile", "command": command_index, "target": error["target"]["name"],
                         "source": name, "name": error["message"]["message"], "code": code.get("code"),
                         "spans": spans})
    return failures


def cargo_record(result, directory, command_index, context=None):
    if type(result.get("exit_code")) is not int or type(result.get("timed_out")) is not bool:
        raise ValueError("Cargo process has no explicit exit and timeout status")
    streams = {}
    for stream in ("stdout", "stderr"):
        path = directory / result[stream]
        if not path.resolve().is_relative_to(directory.resolve()):
            raise ValueError("Cargo output leaves its evidence directory")
        raw = path.read_bytes()
        if hashlib.sha256(raw).hexdigest() != result[stream + "_sha256"]:
            raise ValueError("Cargo output digest changed")
        streams[stream] = raw.decode("utf-8", errors="strict")
    messages, output, finished = [], [], None
    for line in streams["stdout"].splitlines():
        if finished is not None:
            output.append(line)
            continue
        if not line.strip():
            continue
        message = json.loads(line)
        if not isinstance(message, dict) or message.get("reason") not in {
                "compiler-artifact", "compiler-message", "build-script-executed", "build-finished"}:
            raise ValueError("unrecognized Cargo build message")
        messages.append(message)
        if message["reason"] == "build-finished":
            if type(message.get("success")) is not bool:
                raise ValueError("Cargo build completion has no boolean status")
            finished = message["success"]
    if finished is None:
        raise ValueError("Cargo build has no terminal message")
    errors = [message for message in messages if message["reason"] == "compiler-message"
              and message.get("message", {}).get("level") == "error"]
    if not finished:
        failures = hidden_compiler_failures(errors, result, context, command_index)
        return {"status": "fail" if failures else "unknown", "cases": [], "compiler_errors": errors,
                "compile_failures": failures,
                "reason": "Cargo compilation did not succeed"}
    if errors:
        raise ValueError("Cargo reports both build success and compiler errors")
    suites, merged = harness_suites(output)
    artifacts = [message for message in messages if message["reason"] == "compiler-artifact"
                 and message.get("profile", {}).get("test") is True and message.get("executable")]
    launches = []
    for line in streams["stderr"].splitlines():
        binary = re.fullmatch(r"\s*Running (.+) \((.+)\)", line)
        doc = re.fullmatch(r"\s*Doc-tests (\S+)", line)
        if binary:
            if any(launch["kind"] == "doctest" for launch in launches):
                raise ValueError("test binary launched after doctests")
            path = native_path(binary[2])
            matches = [artifact for artifact in artifacts if native_path(artifact["executable"]) == path
                       or native_path(artifact["executable"]).endswith("/" + path)]
            if len(matches) != 1:
                raise ValueError("launched binary does not match one Cargo test artifact")
            artifact = matches[0]
            launches.append({"kind": "binary", "name": artifact["target"]["name"],
                             "source": artifact["target"]["src_path"], "executable": artifact["executable"],
                             "package": artifact["package_id"]})
        elif doc:
            origins = {(artifact["package_id"], artifact["target"]["src_path"], artifact.get("manifest_path")) for artifact in messages
                       if artifact["reason"] == "compiler-artifact" and artifact["target"]["name"] == doc[1]
                       and artifact["target"].get("doctest") is True and "lib" in artifact["target"].get("kind", [])}
            tested_packages = {artifact["package_id"] for artifact in artifacts}
            tested_origins = {origin for origin in origins if origin[0] in tested_packages}
            if tested_origins:
                origins = tested_origins
            if len(origins) != 1:
                raise ValueError("doctests do not match one compiled library target")
            origin = next(iter(origins))
            launches.append({"kind": "doctest", "name": doc[1], "source": origin[1], "manifest": origin[2]})
    binaries = [launch for launch in launches if launch["kind"] == "binary"]
    if len({launch["executable"] for launch in binaries}) != len(binaries):
        raise ValueError("the same test binary was launched more than once")
    if result["exit_code"] == 0:
        if {artifact["executable"] for artifact in artifacts} != {launch["executable"] for launch in binaries}:
            raise ValueError("successful Cargo run omitted a compiled test binary")
        expected_docs = {artifact["target"]["name"] for artifact in artifacts
                         if artifact["target"].get("doctest") is True and "lib" in artifact["target"].get("kind", [])}
        actual_docs = {launch["name"] for launch in launches if launch["kind"] == "doctest"}
        if not expected_docs.issubset(actual_docs):
            raise ValueError("successful Cargo run omitted a selected library's doctests")
    cases, position = [], 0
    for launch_index, launch in enumerate(launches, 1):
        count = 1
        if launch["kind"] == "doctest" and position + 2 in merged and position + 1 not in merged:
            count = 2
        if position + count > len(suites):
            raise ValueError("launched target has no complete test suite")
        for suite in suites[position:position + count]:
            for case in suite["cases"]:
                record = {**case, "command": command_index, "target_index": launch_index,
                          "target": launch["name"], "kind": launch["kind"]}
                if context is not None:
                    root = context["test_root"]
                    record["source"] = source_path(launch["source"], root, root)
                    if launch["kind"] == "doctest":
                        match = re.fullmatch(r"(.+) - .*\(line (\d+)\)(?: - compile(?: fail)?)?", case["name"])
                        if not match or not launch["manifest"]:
                            raise ValueError("doctest has no native file, line or package origin")
                        manifest = Path(launch["manifest"]).parent
                        package_path = manifest.relative_to(root).as_posix()
                        base = root if package_path == "." or match[1].startswith(package_path + "/") else manifest
                        name = source_path(match[1], root, base)
                        line = int(match[2])
                        record.update(source=name, line=line)
                        record["additional"] = any(span["first_line"] <= line <= span["last_line"]
                                                   for span in context.get("additional_doc_ranges", {}).get(name, []))
                cases.append(record)
        position += count
        if position in merged:
            if launch["kind"] != "doctest":
                raise ValueError("merged doctest marker belongs to a native test binary")
            merged.remove(position)
    if position != len(suites) or merged:
        raise ValueError("test suites do not match Cargo's launched targets")
    executed = [case for case in cases if case["status"] != "ignored"]
    failed = any(case["status"] == "FAILED" for case in cases)
    status = "unknown"
    if not result["timed_out"] and executed:
        if result["exit_code"] == 0 and not failed:
            status = "pass"
        elif result["exit_code"] == 101 and failed:
            status = "fail"
    return {"status": status, "cases": cases, "compiler_errors": errors}


def rust_status(results, directory, context=None):
    commands, invalid = [], []
    for number, result in enumerate(results, 1):
        try:
            command = {**cargo_record(result, directory, number, context), "command": number}
            if context is not None and command["status"] != "unknown":
                original = [case for case in command["cases"] if not case.get("additional")]
                if not command.get("compile_failures"):
                    if not any(case["status"] != "ignored" for case in original):
                        command["status"] = "unknown"
                    elif command["status"] == "fail" and not any(case["status"] == "FAILED" for case in original):
                        command["status"] = "unknown"
                        command["reason"] = "added doctest failed; existing test completion is unverified"
            commands.append(command)
        except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
            invalid.append({"command": number, "reason": str(error)})
    all_cases = [case for command in commands for case in command["cases"]]
    cases = [case for case in all_cases if not case.get("additional")]
    status = "unknown"
    if commands and not invalid and all(command["status"] != "unknown" for command in commands):
        status = "fail" if any(command["status"] == "fail" for command in commands) else "pass"
    report = {"status": status, "test_result_events": sum(case["status"] != "ignored" for case in cases),
            "skipped": [case for case in cases if case["status"] == "ignored"],
            "failures": [case for case in cases if case["status"] == "FAILED"] +
                        [failure for command in commands for failure in command.get("compile_failures", [])],
            "compiler_errors": [error for command in commands for error in command["compiler_errors"]],
            "unknown_commands": [{"command": command["command"], "reason": command.get("reason", "test outcome is incomplete")}
                                 for command in commands if command["status"] == "unknown"],
            "invalid_reports": invalid}
    if context is not None:
        report["additional_doc_cases"] = [case for case in all_cases if case.get("additional")]
    return report
