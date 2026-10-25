import hashlib
import base64
import copy
import json
import os
from pathlib import Path
import shutil
import tempfile
import unittest

from rust_results import rust_status
from validate_dataset import run_commands


class RustResultsTests(unittest.TestCase):
    def record(self, directory, body, *, code=0, stderr="", build=True, artifacts=None, timeout=False):
        artifact = {"reason": "compiler-artifact", "profile": {"test": True}, "package_id": "fixture",
                    "executable": "/workspace/target/debug/deps/fixture",
                    "target": {"name": "fixture", "src_path": "/workspace/src/lib.rs", "kind": ["lib"], "doctest": False}}
        messages = [artifact] if artifacts is None else artifacts
        text = "\n".join(json.dumps(value) for value in [*messages, {"reason": "build-finished", "success": build}])
        record = {"exit_code": code, "timed_out": timeout}
        for stream, value in (("stdout", text + "\n" + body),
                              ("stderr", stderr or "     Running unittests src/lib.rs (target/debug/deps/fixture)\n")):
            path = directory / (stream + ".log")
            path.write_text(value)
            record[stream] = path.name
            record[stream + "_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        return record

    def test_targets_counts_failures_and_skips_are_independent(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            for outcome in ("ok", "FAILED"):
                body = (f"running 2 tests\ntest feature::behavior ... {outcome}\ntest platform ... ignored, requires a service\n"
                        f"test result: {outcome}. {int(outcome == 'ok')} passed; {int(outcome == 'FAILED')} failed; "
                        "1 ignored; 0 measured; 0 filtered out; finished in 0.00s\n")
                record = self.record(directory, body, code=0 if outcome == "ok" else 101)
                report = rust_status([record], directory)
                self.assertEqual(report["status"], "pass" if outcome == "ok" else "fail", report)
                self.assertEqual(report["test_result_events"], 1)
                self.assertEqual(report["skipped"][0]["reason"], "requires a service")
                self.assertEqual(len(report["failures"]), int(outcome == "FAILED"))
                record["timed_out"] = True
                self.assertEqual(rust_status([record], directory)["status"], "unknown")

    def test_compiler_failure_requires_existing_pass_and_all_hidden_diagnostic_locations(self):
        error = {"reason": "compiler-message", "target": {"kind": ["test"], "name": "hidden"},
                 "message": {"level": "error", "rendered": "error: missing requested API", "message": "missing requested API",
                             "code": {"code": "E0425"}, "spans": [{"is_primary": True, "file_name": "tests/hidden.rs",
                                                                    "line_start": 1, "line_end": 1}]}}
        context = {"test_root": "/workspace", "existing_passed": True,
                   "baseline_failure_patterns": ["error: missing requested API"],
                   "hidden_inputs": {"tests/hidden.rs": {"before": "", "after": base64.b64encode(b"requested();\n").decode()}}}
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            record = self.record(directory, "", code=101, build=False, artifacts=[error])
            report = rust_status([record], directory, context)
            self.assertEqual(report["status"], "fail", report)
            self.assertEqual(report["test_result_events"], 0)
            self.assertEqual(report["failures"][0]["kind"], "compile")
            for key, value in (("existing_passed", False), ("baseline_failure_patterns", []), ("hidden_inputs", {})):
                with self.subTest(key=key):
                    self.assertEqual(rust_status([record], directory, {**context, key: value})["status"], "unknown")
            for change in ("production", "outside", "not-added", "no-span", "mixed-error", "timeout"):
                with self.subTest(change=change):
                    modified = copy.deepcopy(error)
                    errors = [modified]
                    if change == "production":
                        modified["target"]["kind"] = ["lib"]
                    elif change == "outside":
                        modified["message"]["spans"][0]["file_name"] = "../outside.rs"
                    elif change == "not-added":
                        modified["message"]["spans"][0].update(line_start=2, line_end=2)
                    elif change == "no-span":
                        modified["message"]["spans"] = []
                    elif change == "mixed-error":
                        modified["message"]["spans"][0]["file_name"] = "src/lib.rs"
                        errors.append(error)
                    record = self.record(directory, "", code=101, build=False, artifacts=errors, timeout=change == "timeout")
                    self.assertEqual(rust_status([record], directory, context)["status"], "unknown")

    def test_missing_or_contradictory_native_evidence_never_passes(self):
        body = ("running 1 test\ntest behavior ... ok\n"
                "test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n")
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            variants = {
                "partial": body.split("test result:")[0],
                "mismatch": body.replace("1 passed", "2 passed"),
                "summary_only": body.split("test result:")[1],
                "filter": body.replace("0 filtered out", "1 filtered out"),
                "all_skipped": body.replace("... ok", "... ignored").replace("1 passed", "0 passed").replace("0 ignored", "1 ignored"),
                "duplicate": body.replace("running 1 test", "running 2 tests").replace("test behavior ... ok", "test behavior ... ok\ntest behavior ... ok").replace("1 passed", "2 passed"),
                "extra": body + body,
            }
            for label, text in variants.items():
                with self.subTest(label=label):
                    report = rust_status([self.record(directory, text)], directory)
                    self.assertEqual(report["status"], "unknown", report)
            record = self.record(directory, body, code=101)
            self.assertEqual(rust_status([record], directory)["status"], "unknown")
            record = self.record(directory, body, stderr="no native launch\n")
            self.assertEqual(rust_status([record], directory)["status"], "unknown")
            record = self.record(directory, body)
            (directory / record["stdout"]).write_text(body)
            self.assertEqual(rust_status([record], directory)["status"], "unknown")
            error = {"reason": "compiler-message", "message": {"level": "error", "message": "missing API"}}
            record = self.record(directory, "", code=101, build=False, artifacts=[error])
            report = rust_status([record], directory)
            self.assertEqual(report["status"], "unknown")
            self.assertEqual(report["compiler_errors"], [error])
            record = self.record(directory, "", code=101, build=False,
                                 artifacts=[{"reason": "compiler-message", "message": None}])
            self.assertEqual(rust_status([record], directory)["status"], "unknown")

    def test_edition_2024_doc_suites_keep_one_target_and_distinct_cases(self):
        body = ("running 1 test\ntest src/lib.rs - value (line 2) ... ok\n"
                "test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n"
                "running 1 test\ntest src/lib.rs - value (line 6) - compile fail ... ok\n"
                "test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.03s\n"
                "all doctests ran in 0.49s; merged doctests compilation took 0.17s\n")
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            artifact = {"reason": "compiler-artifact", "profile": {"test": False}, "package_id": "fixture",
                        "target": {"name": "fixture", "src_path": "/workspace/src/lib.rs", "kind": ["lib"], "doctest": True}}
            record = self.record(directory, body, artifacts=[artifact], stderr="   Doc-tests fixture\n")
            report = rust_status([record], directory)
            self.assertEqual(report["status"], "pass", report)
            self.assertEqual(report["test_result_events"], 2)
            record = self.record(directory, body.replace("all doctests ran", "unknown doctest marker"),
                                 artifacts=[artifact], stderr="   Doc-tests fixture\n")
            self.assertEqual(rust_status([record], directory)["status"], "unknown")
            record = self.record(directory, body, artifacts=[], stderr="   Doc-tests fixture\n")
            self.assertEqual(rust_status([record], directory)["status"], "unknown")

    def test_same_named_dependency_does_not_replace_the_tested_library(self):
        body = ("running 1 test\ntest behavior ... ok\n"
                "test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n"
                "running 1 test\ntest src/lib.rs - value (line 2) ... ok\n"
                "test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.03s\n")
        selected = {"reason": "compiler-artifact", "profile": {"test": True}, "package_id": "path+file:///workspace#fixture@0.1.0",
                    "executable": "/workspace/target/debug/deps/fixture",
                    "target": {"name": "fixture", "src_path": "/workspace/src/lib.rs", "kind": ["lib"], "doctest": True}}
        dependency = {"reason": "compiler-artifact", "profile": {"test": False}, "package_id": "registry+https://example.invalid#fixture@0.2.0",
                      "target": {"name": "fixture", "src_path": "/registry/fixture/src/lib.rs", "kind": ["lib"], "doctest": True}}
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            record = self.record(directory, body, artifacts=[dependency, selected],
                                 stderr="     Running unittests src/lib.rs (target/debug/deps/fixture)\n   Doc-tests fixture\n")
            report = rust_status([record], directory)
            self.assertEqual(report["status"], "pass", report)
            self.assertEqual(report["test_result_events"], 2)

    def test_workspace_doctest_compile_identity_maps_to_its_package(self):
        body = ("running 1 test\ntest member/src/lib.rs - value (line 2) - compile ... ok\n"
                "test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n")
        artifact = {"reason": "compiler-artifact", "profile": {"test": False}, "package_id": "fixture",
                    "manifest_path": "/workspace/member/Cargo.toml",
                    "target": {"name": "fixture", "src_path": "/workspace/member/src/lib.rs", "kind": ["lib"], "doctest": True}}
        context = {"test_root": "/workspace", "additional_doc_ranges": {"member/src/lib.rs": [{"first_line": 1, "last_line": 4}]}}
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            record = self.record(directory, body, artifacts=[artifact], stderr="   Doc-tests fixture\n")
            report = rust_status([record], directory, context)
            self.assertEqual(report["status"], "unknown")
            self.assertEqual(report["additional_doc_cases"][0]["source"], "member/src/lib.rs")
            self.assertEqual(report["invalid_reports"], [])
            context["additional_doc_ranges"] = {}
            self.assertEqual(rust_status([record], directory, context)["status"], "pass")
            record = self.record(directory, body.replace("member/src", "src"), artifacts=[artifact], stderr="   Doc-tests fixture\n")
            self.assertEqual(rust_status([record], directory, context)["status"], "pass")

    @unittest.skipUnless(os.environ.get("TASKBELAY_EVAL_RUST_NATIVE") == "1" and shutil.which("cargo"),
                         "native Cargo environment not configured")
    def test_native_cargo_build_tests_and_edition_2024_doctests(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            (root / "src").mkdir()
            (root / "Cargo.toml").write_text('[package]\nname="fixture"\nversion="0.1.0"\nedition="2024"\n'
                                           '[features]\nfailure=[]\ncompile_failure=[]\n')
            (root / "src/lib.rs").write_text('''/// ```
/// assert_eq!(fixture::value(), 2);
/// ```
/// ```compile_fail
/// fixture::missing();
/// ```
pub fn value() -> i32 { 2 }
#[cfg(feature="compile_failure")] pub fn invalid() { missing(); }
#[cfg(test)] mod tests {
    #[test] fn works() { assert_eq!(super::value(), 2); }
    #[test] #[ignore="fixture reason"] fn ignored() {}
    #[cfg(feature="failure")] #[test] fn fails() { assert_eq!(super::value(), 3); }
}
''')
            for feature, expected in ((None, "pass"), ("failure", "fail"), ("compile_failure", "unknown")):
                with self.subTest(feature=feature):
                    directory = root / (feature or "pass")
                    directory.mkdir()
                    argv = ["cargo", "test", "--offline", "--message-format=json"]
                    if feature:
                        argv += ["--features", feature]
                    argv += ["--", "--format=pretty", "--color=never"]
                    records = run_commands([argv], root, directory,
                                           {**os.environ, "CARGO_TERM_COLOR": "never", "CARGO_BUILD_JOBS": "2"}, 90)
                    report = rust_status(records, directory)
                    self.assertEqual(report["status"], expected, report)
                    if feature == "compile_failure":
                        self.assertTrue(report["compiler_errors"])
                    else:
                        self.assertEqual(len(report["skipped"]), 1)
                        self.assertEqual(report["test_result_events"], 2 if feature else 3)


if __name__ == "__main__":
    unittest.main()
