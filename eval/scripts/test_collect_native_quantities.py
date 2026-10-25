import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import collect_native_quantities as native
import dataset


class NativeQuantityTests(unittest.TestCase):
    def test_native_pytest_uses_full_fixed_trees_and_leaves_source_index_untouched(self):
        self.run_fixture()

    def test_native_failure_is_retained_as_unknown(self):
        self.run_fixture(failed=True)

    def test_preparation_cannot_change_fixed_source(self):
        self.run_fixture(change_source=True)

    def run_fixture(self, failed=False, change_source=False):
        python = os.environ.get("TASKBELAY_EVAL_PYTHON", sys.executable)
        version = subprocess.run([python, "-m", "pytest", "--version"], capture_output=True, text=True)
        if version.returncode:
            self.skipTest("native pytest is required for fixed-tree quantity collection")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            source, task = root / "source", root / "task"
            source.mkdir()
            task.mkdir()
            dataset.git(source, "init", "--initial-branch=main")

            def commit():
                dataset.git(source, "add", "--all")
                dataset.git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                            "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
                return dataset.git(source, "rev-parse", "HEAD").stdout.decode().strip()

            text = ('import pytest\n@pytest.mark.parametrize("n", VALUES, ids=str)\n'
                    'def test_parameter(n): assert n > 0\n')
            (source / "test_cases.py").write_text(text.replace("VALUES", "[1, 1]"))
            base = commit()
            target = text.replace("VALUES", "[1, 1, -1]" if failed else "[1, 1, 2]")
            target += "def test_new(): assert True\n"
            (source / "test_cases.py").write_text(target)
            merged = commit()
            base_tree = dataset.git(source, "rev-parse", base + "^{tree}").stdout.decode().strip()
            tree = dataset.git(source, "rev-parse", merged + "^{tree}").stdout.decode().strip()
            reference = dataset.git(source, "diff", base, merged).stdout
            for name in ("reference.patch", "implementation.patch"):
                (task / name).write_bytes(reference)
            (task / "hidden.patch").write_text("authored hidden acceptance must never be applied here\n")
            (task / "request.md").write_text("Keep the requested behavior.\n")
            manifest = {"id": "native-quantity", "language": "Python", "base_commit": base,
                        "merge_commit": merged, "base_tree": base_tree, "files": [],
                        "sha256": {p.name: dataset.digest(p) for p in task.iterdir()},
                        "checks": {phase: [[python, "-m", "pytest", "-q"]] for phase in ("existing", "hidden")}}
            if change_source:
                manifest["checks"]["prepare"] = [[python, "-c", "from pathlib import Path; Path('test_cases.py').write_text('')"]]
            (task / "task.json").write_text(json.dumps(manifest))
            static = root / "static"
            static.mkdir()
            static_path = static / "quantities.json"
            static_path.write_text(json.dumps({"candidate": {"base_tree": base_tree, "tree": tree,
                "test_migrations": {}, "unknown": [{"file": "test_cases.py", "metric": "tests",
                                                     "reason": "变化的用例无法完整枚举: test_parameter"}]}}))
            # The source working directory deliberately differs from both trees.
            (source / "test_cases.py").write_text("raise RuntimeError('uncommitted user content')\n")
            before = native.repository_state(source)
            runtime = {"versions": [[python, "-m", "pytest", "--version"]], "timeout_seconds": 30}
            output = root / "result"
            report = native.collect(task, source, source, tree, static_path, output, runtime)
            self.assertEqual(native.repository_state(source), before)
            self.assertEqual((source / "test_cases.py").read_text(), "raise RuntimeError('uncommitted user content')\n")
            self.assertTrue(report["repositories_unchanged"])
            self.assertFalse(report["hidden_tests_added"])
            self.assertTrue((output / "result.json").is_file())
            if failed or change_source:
                self.assertFalse(report["complete"])
                self.assertIsNone(report["native_added_test_count"])
                self.assertIn("error", report)
                return
            self.assertTrue(report["complete"], report.get("error"))
            self.assertEqual(report["variants"]["base"]["test_count"], 2)
            self.assertEqual(report["variants"]["target"]["test_count"], 4)
            self.assertEqual(report["native_added_test_count"], 2)
            self.assertEqual((output / "target/workspace/test_cases.py").read_text(), target)
            self.assertFalse((output / "target/hidden").exists())
            revised = native.reassess(output / "result.json", root / "reassessed.json")
            self.assertTrue(revised["complete"], revised["unknown"])
            self.assertEqual(revised["native_added_test_count"], 2)
            original_result = (output / "result.json").read_bytes()
            (output / "target/existing/1.stdout.log").write_text("changed evidence")
            invalid = native.reassess(output / "result.json", root / "invalid-reassessment.json")
            self.assertFalse(invalid["complete"])
            self.assertIsNone(invalid["native_added_test_count"])
            self.assertIn("recorded evidence changed", invalid["unknown"][0])
            self.assertEqual((output / "result.json").read_bytes(), original_result)
            with self.assertRaisesRegex(ValueError, "match the fixed"):
                native.collect(task, source, source, base_tree, static_path, root / "wrong-tree", runtime)
            self.assertFalse((root / "wrong-tree").exists())

    def test_duplicate_names_keep_multiplicity_and_unmapped_migrations_stay_unknown(self):
        before = [{"package": "test.ts", "name": "same", "occurrences": 2}]
        after = [{"package": "test.ts", "name": "same", "occurrences": 3}]
        static = {"test_migrations": {}, "unknown": []}
        self.assertEqual(native.difference(before, after, static)["native_added_test_count"], 1)
        self.assertEqual(native.difference(after, before, static)["native_added_test_count"], 0)
        self.assertEqual(native.difference(before, before, static)["native_added_test_count"], 0)
        static["test_migrations"] = {"old": "new"}
        self.assertIsNone(native.difference(before, after, static)["native_added_test_count"])
        static = {"test_migrations": {}, "unknown": [{"file": "test.ts", "metric": "tests", "reason": "unparsed file"}]}
        self.assertIsNone(native.difference(before, after, static)["native_added_test_count"])

    def test_removed_and_added_names_do_not_prove_new_cases_or_a_net_count(self):
        before = [{"package": "test.py", "name": "old[argument]", "occurrences": 1}]
        after = [{"package": "test.py", "name": "new[argument]", "occurrences": 1},
                 {"package": "test.py", "name": "new[option]", "occurrences": 1}]
        result = native.difference(before, after, {"test_migrations": {}, "unknown": []})
        self.assertIsNone(result["native_added_test_count"])
        self.assertTrue(result["unknown"])
        self.assertEqual(sum(item["occurrences"] for item in result["added_native_identities"]), 2)
        self.assertEqual(sum(item["occurrences"] for item in result["removed_native_identities"]), 1)

    def test_junit_keeps_duplicate_instances_and_rejects_overlap_or_tampered_report(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / "report.xml"
            path.write_text('<testsuites><testsuite tests="2" failures="0" errors="0" skipped="0">'
                            '<testcase classname="test_parameter" name="test_named[中文]"/>'
                            '<testcase classname="test_parameter" name="test_named[中文]"/>'
                            '</testsuite></testsuites>')
            record = {"cwd": ".", "test_report": path.name, "test_report_sha256": dataset.digest(path),
                      "exit_code": 0, "timed_out": False}
            cases = native.native_cases("Python", [record], root)
            self.assertEqual(cases, [{"package": ".:test_parameter", "name": "test_named[中文]", "occurrences": 2}])
            with self.assertRaisesRegex(ValueError, "overlap"):
                native.native_cases("Python", [record, record], root)
            path.write_text("<incomplete")
            with self.assertRaisesRegex(ValueError, "did not finish"):
                native.native_cases("Python", [record], root)

    def test_vitest_keeps_duplicate_names_within_full_report(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / "report.json"
            report = {"success": True, "numTotalTests": 2, "numPassedTests": 2, "numFailedTests": 0,
                      "numPendingTests": 0, "numTodoTests": 0, "numTotalTestSuites": 1,
                      "numPassedTestSuites": 1, "numFailedTestSuites": 0, "numPendingTestSuites": 0,
                      "testResults": [{"name": str(root / "src/record.test.ts"), "status": "passed",
                                       "assertionResults": [{"status": "passed", "fullName": "async parsing"}] * 2}]}
            path.write_text(json.dumps(report))
            record = {"cwd": ".", "test_report": path.name, "test_report_sha256": dataset.digest(path),
                      "test_root": str(root), "exit_code": 0, "timed_out": False}
            self.assertEqual(native.native_cases("TypeScript", [record], root),
                             [{"package": "src/record.test.ts", "name": "async parsing", "occurrences": 2}])

    def test_vitest_repeat_modes_and_typechecks_do_not_multiply_source_cases(self):
        self.vitest_modes(False)

    def test_vitest_different_mode_inventories_remain_unknown(self):
        self.vitest_modes(True)

    def vitest_modes(self, different):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            runtime = {"name": str(root / "src/record.test.ts"), "status": "passed",
                       "assertionResults": [{"status": "passed", "fullName": "same", "title": "same", "ancestorTitles": []}] * 2}
            second = json.loads(json.dumps(runtime))
            if different:
                second["assertionResults"][0]["fullName"] = "different"
            typed = {"name": runtime["name"], "status": "passed", "assertionResults": [
                {"status": "passed", "fullName": "src/record.test.ts same", "title": "same",
                 "ancestorTitles": ["src/record.test.ts"], "meta": {"typecheck": True}}] * 2 + [
                {"status": "passed", "fullName": "src/record.test.ts", "title": "",
                 "ancestorTitles": ["src/record.test.ts"], "meta": {"typecheck": True}}]}
            only_type = {"name": str(root / "src/only.test-d.ts"), "status": "passed", "assertionResults": [
                {"status": "passed", "fullName": "src/only.test-d.ts type contract", "title": "type contract",
                 "ancestorTitles": ["src/only.test-d.ts"], "meta": {"typecheck": True}}]}
            report = {"success": True, "numTotalTests": 8, "numPassedTests": 8, "numFailedTests": 0,
                      "numPendingTests": 0, "numTodoTests": 0, "numTotalTestSuites": 4,
                      "numPassedTestSuites": 4, "numFailedTestSuites": 0, "numPendingTestSuites": 0,
                      "testResults": [runtime, second, typed, only_type]}
            path = root / "report.json"
            path.write_text(json.dumps(report))
            record = {"cwd": ".", "test_report": path.name, "test_report_sha256": dataset.digest(path),
                      "test_root": str(root), "exit_code": 0, "timed_out": False}
            if different:
                with self.assertRaisesRegex(ValueError, "inventories differ"):
                    native.native_cases("TypeScript", [record], root)
            else:
                self.assertEqual(native.native_cases("TypeScript", [record], root), [
                    {"package": "src/only.test-d.ts", "name": "type contract", "occurrences": 1},
                    {"package": "src/record.test.ts", "name": "same", "occurrences": 2}])


if __name__ == "__main__":
    unittest.main()
