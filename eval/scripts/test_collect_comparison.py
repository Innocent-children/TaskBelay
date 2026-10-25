import copy
import json
from pathlib import Path
import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET

import collect_comparison
import dataset
from assess_candidate import TEST_RESULT_FORMATS, rust_context, test_status
from run_agents import integration, parse_events


class ComparisonTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name).resolve()
        self.manifest = {"schema_version": 1, "dataset": "index.json", "series": [{
            "id": "claude-fixed", "agent": "claude", "version": "fixture-version", "model": "fixture-model",
            "platform": "fixture-platform", "dynamic_checks": False,
            "configuration": {mode: {"arguments": [mode], "settings": []} for mode in ("off", "on")}}], "attempts": []}
        self.write(self.root / "index.json", {"tasks": [{"id": "fixture"}]})

    def write(self, path, value):
        path.write_text(json.dumps(value), encoding="utf-8")

    def attempt(self, mode, replicate=1, phase="main", hidden="pass", existing="pass", integrated=True, failure="TestHidden", language="Go"):
        name = f"attempt-{len(self.manifest['attempts']) + 1}"
        agent, assessment = self.root / (name + "-agent"), self.root / (name + "-assessment")
        agent.mkdir()
        assessment.mkdir()
        metadata = {"agent": "claude", "version": "fixture-version", "model": "fixture-model",
                    "platform": "fixture-platform", "task_id": "fixture", "enabled": mode == "on",
                    "workspace": name, "started_at": len(self.manifest["attempts"]),
                    **self.manifest["series"][0]["configuration"][mode]}
        self.write(agent / "metadata.json", metadata)
        events = [{"type": "system", "subtype": "init", "model": "fixture-model", "session_id": name},
                  {"type": "assistant", "session_id": name, "message": {"content": [{"type": "tool_use"}]}},
                  {"type": "result", "subtype": "success", "session_id": name, "usage": {}}]
        (agent / "turn-1.jsonl").write_text("".join(json.dumps(event) + "\n" for event in events))
        parsed = parse_events(agent / "turn-1.jsonl", "claude")
        hooks = [{"kind": kind, "state": "recorded", "error": "", "session_id": name}
                 for kind in ("session_start", "user_prompt", "edit_use", "stop")] if mode == "on" and integrated else []
        failures = integration(mode == "on", hooks, parsed, None, 0, False)
        receipt = {"valid_integration": not failures, "turns": [{"turn": 1, "session_id": name, "events": parsed,
                   "version": metadata["version"], "version_after": metadata["version"], "settings": [], "settings_after": [],
                   "requested_model": metadata["model"], "hooks": hooks, "unmatched_hooks": [], "returncode": 0,
                   "timed_out": False, "duration_seconds": 1, "valid_integration": not failures, "failures": failures}]}
        self.write(agent / "result.json", receipt)
        material = assessment / "task-material"
        material.mkdir()
        for artifact in ("request.md", "reference.patch", "implementation.patch", "hidden.patch"):
            (material / artifact).write_text("fixed material for " + artifact)
        self.write(material / "task.json", {"id": "fixture", "language": language, "base_commit": "a" * 40,
                   "base_tree": "b" * 40, "merge_commit": "c" * 40, "checks": {},
                   "sha256": {path.name: dataset.digest(path) for path in material.iterdir()}})
        if not (self.root / "fixture").exists():
            shutil.copytree(material, self.root / "fixture")
        (assessment / "candidate.patch").write_text("candidate " + name)
        report = {"task_id": "fixture", "valid_integration": not failures, "correctness_evaluated": True,
                  "test_result_format": TEST_RESULT_FORMATS[language],
                  "valid_correctness_sample": not failures, "agent_metadata_sha256": dataset.digest(agent / "metadata.json"),
                  "agent_result_sha256": dataset.digest(agent / "result.json"),
                  "candidate_patch_sha256": dataset.digest(assessment / "candidate.patch")}
        if language == "Rust":
            report["rust_test_root"] = str(assessment / "validation")
            protection = assessment / "rust-test-inputs"
            protection.mkdir()
            for file in ("input.json", "stdout.json"):
                self.write(protection / file, [])
            proof = {"input_sha256": dataset.digest(protection / "input.json"),
                     "output_sha256": dataset.digest(protection / "stdout.json"), "additional_doc_ranges": {}}
            self.write(protection / "result.json", proof)
            report["rust_protection"] = {**proof, "result_sha256": dataset.digest(protection / "result.json")}
            self.write(assessment / "rust-hidden-inputs.json", {})
            report["rust_hidden_inputs_sha256"] = dataset.digest(assessment / "rust-hidden-inputs.json")
        for kind, status in (("existing", existing), ("hidden", hidden)):
            directory = assessment / kind
            directory.mkdir()
            event = {"Action": status, "Package": "fixture", "Test": failure if status == "fail" else "TestPass"}
            (directory / "stdout.log").write_text(json.dumps(event) + "\n")
            (directory / "stderr.log").write_text("")
            command = {"stdout": "stdout.log", "stderr": "stderr.log", "timed_out": False,
                       "exit_code": 1 if status == "fail" else 0,
                       "stdout_sha256": dataset.digest(directory / "stdout.log"),
                       "stderr_sha256": dataset.digest(directory / "stderr.log")}
            if language == "Python":
                suite = ET.Element("testsuite", tests="1", failures=str(int(status == "fail")), errors="0",
                                   skipped=str(int(status == "skip")))
                case = ET.SubElement(suite, "testcase", classname="fixture", name=event["Test"])
                if status != "pass":
                    ET.SubElement(case, "failure" if status == "fail" else "skipped", message="fixture")
                xml = directory / "pytest.xml"
                ET.ElementTree(suite).write(xml, encoding="utf-8", xml_declaration=True)
                command.update(test_report=xml.name, test_report_sha256=dataset.digest(xml))
            if language == "TypeScript":
                native = directory / "vitest.json"
                failed, skipped = status == "fail", status == "skip"
                self.write(native, {"numTotalTestSuites": 1, "numPassedTestSuites": int(not failed),
                    "numFailedTestSuites": int(failed), "numPendingTestSuites": 0,
                    "numTotalTests": 1, "numPassedTests": int(status == "pass"), "numFailedTests": int(failed),
                    "numPendingTests": int(skipped), "numTodoTests": 0, "success": not failed,
                    "testResults": [{"name": str(assessment / "validation/value.test.ts"),
                        "status": "failed" if failed else "passed", "message": "", "assertionResults": [{
                            "fullName": event["Test"], "status": {"pass": "passed", "fail": "failed", "skip": "skipped"}[status],
                            "failureMessages": ["fixture failure"] if failed else []}]}]})
                command.update(test_report=native.name, test_report_sha256=dataset.digest(native),
                               test_root=str(assessment / "validation"))
            if language == "Java":
                reports = directory / "reports"
                reports.mkdir()
                suite = ET.Element("testsuite", name="ValueTest", tests="1", failures=str(int(status == "fail")),
                                   errors="0", skipped=str(int(status == "skip")))
                case = ET.SubElement(suite, "testcase", classname="ValueTest", name=event["Test"])
                if status != "pass":
                    ET.SubElement(case, "failure" if status == "fail" else "skipped", message="fixture")
                xml = reports / "TEST-ValueTest.xml"
                ET.ElementTree(suite).write(xml, encoding="utf-8", xml_declaration=True)
                (directory / "stdout.log").write_text(
                    f"[INFO] Tests run: 1, Failures: {int(status == 'fail')}, Errors: 0, Skipped: {int(status == 'skip')}\n"
                    + ("[ERROR] BUILD FAILURE" if status == "fail" else "[INFO] BUILD SUCCESS") + "\n")
                command.update(test_report_directory="reports", test_report_diagnostics=[],
                               test_reports=[{"path": "reports/" + xml.name, "sha256": dataset.digest(xml)}],
                               stdout_sha256=dataset.digest(directory / "stdout.log"))
            if language == "Rust":
                native_status = {"pass": "ok", "fail": "FAILED", "skip": "ignored"}[status]
                target = str(assessment / "validation")
                artifact = {"reason": "compiler-artifact", "profile": {"test": True}, "package_id": "fixture",
                            "executable": target + "/target/debug/deps/fixture",
                            "target": {"name": "fixture", "src_path": target + "/src/lib.rs", "kind": ["lib"], "doctest": False}}
                (directory / "stdout.log").write_text(json.dumps(artifact) + "\n" + json.dumps({"reason": "build-finished", "success": True}) + "\n"
                    f"running 1 test\ntest {event['Test']} ... {native_status}\n"
                    f"test result: {'FAILED' if status == 'fail' else 'ok'}. {int(status == 'pass')} passed; {int(status == 'fail')} failed; "
                    f"{int(status == 'skip')} ignored; 0 measured; 0 filtered out; finished in 0.00s\n")
                (directory / "stderr.log").write_text("     Running unittests src/lib.rs (target/debug/deps/fixture)\n")
                command.update(exit_code=101 if status == "fail" else 0,
                               stdout_sha256=dataset.digest(directory / "stdout.log"), stderr_sha256=dataset.digest(directory / "stderr.log"))
            context = rust_context(report, assessment, dataset.read_task(material), kind) if language == "Rust" else None
            report[kind] = {**test_status([command], directory, language, context), "commands": [command]}
            if report[kind]["status"] == "unknown":
                report["correctness_evaluated"] = report["valid_correctness_sample"] = False
        self.write(assessment / "result.json", report)
        entry = {"id": name, "series": "claude-fixed", "task_id": "fixture", "phase": phase, "replicate": replicate,
                 "agent_directory": agent.name, "assessment_directory": assessment.name}
        self.manifest["attempts"].append(entry)
        return entry

    def collect(self):
        path = self.root / "comparison.json"
        self.write(path, self.manifest)
        return collect_comparison.collect(path)

    def main_pair(self):
        for mode in ("off", "on"):
            for repeat in range(1, 4):
                self.attempt(mode, repeat, hidden="fail" if mode == "on" and repeat == 3 else "pass")

    def test_usage_gaps_do_not_fabricate_cost_or_discard_correctness(self):
        entry = self.attempt("off")
        result = self.collect()
        row = result["attempts"][0]
        self.assertTrue(row["eligible"])
        self.assertTrue(row["usage"]["valid_evidence"])
        self.assertEqual(row["usage"]["invocation_wall_seconds"], 1)
        self.assertIsNone(row["usage"]["agent_verification_invocations"])
        self.assertIsNone(row["usage"]["model_tokens"]["native_field_totals"]["input_tokens"])
        self.assertEqual(row["usage"]["command_trace"]["status"], "unknown")
        self.assertEqual(result["adoption"]["status"], "not_evaluated")
        agent = self.root / entry["agent_directory"]
        receipt = json.loads((agent / "result.json").read_text())
        receipt["command_trace"] = ["malformed"]
        self.write(agent / "result.json", receipt)
        assessment = self.root / entry["assessment_directory"] / "result.json"
        report = json.loads(assessment.read_text())
        report["agent_result_sha256"] = dataset.digest(agent / "result.json")
        self.write(assessment, report)
        row = self.collect()["attempts"][0]
        self.assertTrue(row["eligible"])
        self.assertEqual(row["usage"]["command_trace"]["status"], "unknown")

    def test_typescript_native_report_is_recomputed_and_tampering_is_rejected(self):
        off = self.attempt("off", language="TypeScript")
        on = self.attempt("on", hidden="fail", failure="type contract", language="TypeScript")
        report = self.collect()
        self.assertTrue(all(row["eligible"] for row in report["attempts"]))
        self.assertEqual(report["attempts"][1]["failures"]["hidden"], [("value.test.ts", "type contract", "failed", 0, False)])
        native = self.root / off["assessment_directory"] / "existing/vitest.json"
        value = json.loads(native.read_text()); value["numTotalTests"] = 99
        self.write(native, value)
        self.assertFalse(self.collect()["attempts"][0]["eligible"])
        path = self.root / on["assessment_directory"] / "result.json"
        value = json.loads(path.read_text()); value["hidden"]["status"] = "pass"
        self.write(path, value)
        self.assertFalse(any(row["eligible"] for row in self.collect()["attempts"]))

    def test_java_native_reports_and_console_summary_are_recomputed(self):
        off = self.attempt("off", language="Java")
        on = self.attempt("on", hidden="fail", failure="expected(String)[1]", language="Java")
        report = self.collect()
        self.assertTrue(all(row["eligible"] for row in report["attempts"]))
        self.assertEqual(report["attempts"][1]["failures"]["hidden"], [("ValueTest", "expected(String)[1]")])
        native = self.root / off["assessment_directory"] / "existing/reports/TEST-ValueTest.xml"
        native.write_text("changed")
        self.assertFalse(self.collect()["attempts"][0]["eligible"])
        assessment = self.root / on["assessment_directory"]
        stdout = assessment / "hidden/stdout.log"
        stdout.write_text("[INFO] Tests run: 2, Failures: 1, Errors: 0, Skipped: 0\n[INFO] BUILD FAILURE\n")
        value = json.loads((assessment / "result.json").read_text())
        value["hidden"]["commands"][0]["stdout_sha256"] = dataset.digest(stdout)
        self.write(assessment / "result.json", value)
        self.assertFalse(any(row["eligible"] for row in self.collect()["attempts"]))

    def test_rust_target_failures_and_protection_evidence_are_recomputed(self):
        off = self.attempt("off", language="Rust")
        on = self.attempt("on", hidden="fail", failure="expected_behavior", language="Rust")
        report = self.collect()
        self.assertTrue(all(row["eligible"] for row in report["attempts"]), report["attempts"])
        self.assertEqual(report["attempts"][1]["failures"]["hidden"],
                         [("binary", 1, "fixture", "src/lib.rs", "expected_behavior", "")])
        native = self.root / off["assessment_directory"] / "rust-test-inputs/result.json"
        self.write(native, {"additional_doc_ranges": {"src/lib.rs": [{"first_line": 1, "last_line": 99}]}})
        self.assertFalse(self.collect()["attempts"][0]["eligible"])
        path = self.root / on["assessment_directory"] / "result.json"
        value = json.loads(path.read_text()); value["hidden"]["status"] = "pass"
        self.write(path, value)
        self.assertFalse(any(row["eligible"] for row in self.collect()["attempts"]))

    def test_rust_native_root_survives_evidence_relocation(self):
        entry = self.attempt("off", language="Rust", hidden="fail", failure="expected_behavior")
        before = self.collect()["attempts"][0]
        self.assertTrue(before["eligible"], before["errors"])
        original = self.root / entry["assessment_directory"]
        archived = self.root / ("archived-" + original.name)
        original.rename(archived)
        entry["assessment_directory"] = archived.name
        after = self.collect()["attempts"][0]
        self.assertTrue(after["eligible"], after["errors"])
        self.assertEqual((after["existing"], after["hidden"]), ("pass", "fail"))
        self.assertEqual(after["failures"], before["failures"])
        report = json.loads((archived / "result.json").read_text())
        del report["rust_test_root"]
        self.write(archived / "result.json", report)
        self.assertFalse(self.collect()["attempts"][0]["eligible"])

    def test_rust_declared_root_requires_an_absolute_path(self):
        entry = self.attempt("off", language="Rust")
        path = self.root / entry["assessment_directory"] / "result.json"
        report = json.loads(path.read_text())
        for value in ("relative/validation", "", None, 42):
            with self.subTest(root=value):
                report["rust_test_root"] = value
                self.write(path, report)
                row = self.collect()["attempts"][0]
                self.assertFalse(row["eligible"])
                self.assertTrue(any("Rust native test root" in error for error in row["errors"]))
        del report["rust_test_root"]
        self.write(path, report)
        self.assertTrue(self.collect()["attempts"][0]["eligible"])

    def test_typescript_same_names_keep_type_case_and_file_failures_distinct(self):
        on_entries = []
        for mode in ("off", "on"):
            for repeat in range(1, 4):
                entry = self.attempt(mode, repeat, hidden="fail", failure="", language="TypeScript")
                if mode == "on":
                    on_entries.append(entry)
        self.assertFalse(self.collect()["series"][0]["tasks"][0]["changes"])
        for change in ("typecheck", "case_index", "file"):
            with self.subTest(change=change):
                for entry in on_entries:
                    assessment = self.root / entry["assessment_directory"]
                    path = assessment / "hidden/vitest.json"
                    native = json.loads(path.read_text())
                    failed = {"fullName": "", "status": "failed", "failureMessages": ["fixture failure"]}
                    cases = [failed]
                    if change == "typecheck":
                        failed["meta"] = {"typecheck": True}
                    elif change == "case_index":
                        cases.insert(0, {"fullName": "", "status": "passed", "failureMessages": []})
                    else:
                        cases = []
                    native["testResults"][0]["assertionResults"] = cases
                    native["numTotalTests"] = len(cases)
                    native["numPassedTests"] = int(change == "case_index")
                    native["numFailedTests"] = int(change != "file")
                    self.write(path, native)
                    report = json.loads((assessment / "result.json").read_text())
                    commands = report["hidden"]["commands"]
                    commands[0]["test_report_sha256"] = dataset.digest(path)
                    report["hidden"] = {**test_status(commands, assessment / "hidden", "TypeScript"),
                                        "commands": commands}
                    self.write(assessment / "result.json", report)
                report = self.collect()
                self.assertTrue(all(row["eligible"] for row in report["attempts"]))
                task = report["series"][0]["tasks"][0]
                self.assertEqual(task["changes"][0]["off_passes"], task["changes"][0]["on_passes"])
                self.assertTrue(task["changes"][0]["failure_tests_changed"])
                self.assertEqual(len(task["additional_runs_missing"]), 6)

    def test_one_failure_is_a_regression_and_requires_six_followups(self):
        self.main_pair()
        report = self.collect()
        group, task = report["series"][0], report["series"][0]["tasks"][0]
        self.assertEqual(group["completed_main_slots"], 6)
        self.assertEqual(task["main"]["on"]["hidden"]["pass_rate"], 2 / 3)
        self.assertEqual(task["changes"], [{"tests": "hidden", "off_passes": 3, "on_passes": 2,
                                           "regression": True, "failure_tests_changed": True}])
        self.assertEqual(group["overall_main_correctness"]["on"]["hidden"]["pass_rate"], 2 / 3)
        self.assertEqual(group["overall_main_correctness"]["on"]["hidden"]["failed_tasks"], ["fixture"])
        self.assertEqual(len(task["additional_runs_missing"]), 6)
        self.assertEqual(report["adoption"]["status"], "not_evaluated")
        self.assertEqual(report["unconfigured_agents"], ["codex"])
        for mode in ("off", "on"):
            for repeat in range(1, 4):
                self.attempt(mode, repeat, "followup", hidden="fail" if mode == "on" else "pass")
        task = self.collect()["series"][0]["tasks"][0]
        self.assertEqual(task["additional_runs_missing"], [])
        self.assertEqual(task["followup"]["on"]["hidden"]["failures"], 3)
        self.assertTrue(task["failure_review_required"])

    def test_retries_keep_invalid_attempts_but_cannot_replace_valid_failures(self):
        self.attempt("on", integrated=False)
        self.attempt("on", hidden="fail")
        first = self.collect()["series"][0]
        self.assertEqual(first["attempt_counts"]["main"]["integration_failure_rate"], 0.5)
        self.assertEqual(first["completed_main_slots"], 1)
        self.assertEqual(first["tasks"][0]["main"]["on"]["hidden"]["failures"], 1)
        self.attempt("on", hidden="pass")
        final = self.collect()["series"][0]
        self.assertEqual(final["completed_main_slots"], 0)
        self.assertEqual(final["tasks"][0]["main"]["on"]["slots"][0]["status"], "conflict")
        self.assertIsNone(final["tasks"][0]["main"]["on"]["hidden"]["pass_rate"])

    def test_pilots_are_separate_and_duplicate_receipts_cannot_fill_formal_slots(self):
        entry = self.attempt("off", phase="pilot")
        self.attempt("on", phase="pilot")
        report = self.collect()
        self.assertEqual(report["series"][0]["completed_main_slots"], 0)
        self.assertEqual(report["series"][0]["attempt_counts"]["pilot"]["eligible_attempts"], 2)
        self.manifest["attempts"].append({**entry, "id": "relabelled-pilot", "phase": "main"})
        report = self.collect()
        self.assertFalse(report["attempts"][0]["eligible"])
        self.assertFalse(report["attempts"][2]["eligible"])
        self.assertEqual(report["series"][0]["completed_main_slots"], 0)

    def test_changed_missing_unknown_and_foreign_evidence_never_fills_slots(self):
        changed = self.attempt("off")
        (self.root / changed["assessment_directory"] / "hidden/stdout.log").write_text("changed")
        missing = self.attempt("on")
        missing["assessment_directory"] = None
        self.attempt("on", 2, hidden="skip")
        foreign = self.attempt("off", 2)
        foreign["assessment_directory"] = changed["assessment_directory"]
        report = self.collect()
        self.assertTrue(all(not row["eligible"] and row["errors"] for row in report["attempts"]))
        self.assertEqual(report["series"][0]["completed_main_slots"], 0)
        self.assertIsNone(report["series"][0]["tasks"][0]["main"]["off"]["hidden"]["pass_rate"])

    def test_different_failures_trigger_followups_even_with_equal_pass_counts(self):
        for mode in ("off", "on"):
            for repeat in range(1, 4):
                self.attempt(mode, repeat, hidden="fail", failure="TestBefore" if mode == "off" else "TestAfter")
        task = self.collect()["series"][0]["tasks"][0]
        self.assertEqual(task["changes"][0]["off_passes"], task["changes"][0]["on_passes"])
        self.assertTrue(task["changes"][0]["failure_tests_changed"])
        self.assertEqual(len(task["additional_runs_missing"]), 6)

    def test_python_native_reports_are_recomputed_and_changed_xml_cannot_fill_slots(self):
        off = self.attempt("off", language="Python")
        on = self.attempt("on", hidden="fail", language="Python")
        report = self.collect()
        self.assertEqual(report["series"][0]["completed_main_slots"], 2)
        self.assertEqual(report["attempts"][1]["hidden"], "fail")
        assessment = self.root / on["assessment_directory"]
        (assessment / "hidden/pytest.xml").write_text("changed native evidence")
        report = self.collect()
        self.assertFalse(report["attempts"][1]["eligible"])
        self.assertEqual(report["series"][0]["completed_main_slots"], 1)
        self.assertIn("evidence changed", report["attempts"][1]["errors"][0])
        assessment = self.root / off["assessment_directory"]
        fabricated = json.loads((assessment / "result.json").read_text())
        fabricated["hidden"]["test_result_events"] = 999
        self.write(assessment / "result.json", fabricated)
        report = self.collect()
        self.assertFalse(report["attempts"][0]["eligible"])
        self.assertIn("raw output", report["attempts"][0]["errors"][0])

    def test_fixed_settings_and_dynamic_series_do_not_mix(self):
        self.attempt("off")
        enabled = self.attempt("on")
        other = copy.deepcopy(self.manifest["series"][0])
        other.update(id="claude-dynamic", dynamic_checks=True)
        self.manifest["series"].append(other)
        enabled["series"] = other["id"]
        report = self.collect()
        self.assertEqual([item["completed_main_slots"] for item in report["series"]], [1, 1])
        self.assertEqual(report["series"][0]["tasks"][0]["changes"], [])
        other["configuration"]["on"]["arguments"] = ["unexpected-setting"]
        report = self.collect()
        self.assertEqual([item["completed_main_slots"] for item in report["series"]], [1, 0])
        self.assertIn("fixed series", report["attempts"][1]["errors"][0])

    def test_changed_dataset_and_fabricated_agent_summary_are_not_valid_evidence(self):
        changed = self.attempt("off")
        summary = self.attempt("on")
        material = self.root / changed["assessment_directory"] / "task-material/task.json"
        data = json.loads(material.read_text())
        data["base_tree"] = "d" * 40
        self.write(material, data)
        raw = self.root / summary["agent_directory"] / "turn-1.jsonl"
        raw.write_text('{"type":"result","subtype":"error"}\n')
        report = self.collect()
        self.assertFalse(any(row["eligible"] for row in report["attempts"]))
        self.assertIn("selected dataset", report["attempts"][0]["errors"][0])
        self.assertIn("native output", report["attempts"][1]["errors"][0])


if __name__ == "__main__":
    unittest.main()
