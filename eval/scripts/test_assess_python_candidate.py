import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import assess_candidate
import dataset


class PythonCandidateAssessmentTests(unittest.TestCase):
    def test_native_pytest_preserves_original_assertions_fixtures_and_agent_workspace(self):
        python = os.environ.get("TASKBELAY_EVAL_PYTHON", sys.executable)
        version = subprocess.run([python, "-m", "pytest", "--version"], capture_output=True, text=True)
        if version.returncode:
            self.skipTest("native pytest is required for Python candidate assessment")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            source, task, agent = (root / name for name in ("source", "task", "agent"))
            for path in (source, task, agent):
                path.mkdir()
            dataset.git(source, "init", "--initial-branch=main")

            def commit():
                dataset.git(source, "add", "--all")
                dataset.git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                            "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
                return dataset.git(source, "rev-parse", "HEAD").stdout.decode().strip()

            (source / "tests").mkdir()
            (source / "src").mkdir()
            (source / "src/value.py").write_text("def value(): return 1\n")
            original = "from value import value\ndef test_existing(threshold): assert value() >= threshold\n"
            fixture = "import pytest\n@pytest.fixture\ndef threshold(): return 0\n"
            (source / "tests/test_value.py").write_text(original)
            (source / "conftest.py").write_text(fixture)
            base = commit()
            (source / "src/value.py").write_text("def value(): return 2\n")
            (source / "tests/test_hidden.py").write_text(
                'import pytest\nfrom value import value\n@pytest.mark.parametrize("expected", [2, 2], ids=["<a&b>", "中文"])\n'
                'def test_hidden(expected): assert value() == expected\n')
            merged = commit()
            for name, paths in (("reference.patch", []), ("implementation.patch", ["src/value.py"]),
                                ("hidden.patch", ["tests/test_hidden.py"])):
                (task / name).write_bytes(dataset.git(source, "diff", base, merged, "--", *paths).stdout)
            (task / "request.md").write_text("Return the requested value.\n")
            manifest = {"id": "python-fixture", "language": "Python", "base_commit": base, "merge_commit": merged,
                        "base_tree": dataset.git(source, "rev-parse", base + "^{tree}").stdout.decode().strip(),
                        "files": [{"path": "src/value.py", "category": "source"}],
                        "sha256": {path.name: dataset.digest(path) for path in task.iterdir()},
                        "checks": {phase: [[python, "-m", "pytest", "-q"]] for phase in ("existing", "hidden")}}
            (task / "task.json").write_text(json.dumps(manifest))
            workspace = root / "workspace"
            dataset.prepare(task, source, workspace)
            (agent / "metadata.json").write_text(json.dumps({"workspace": str(workspace), "task_id": manifest["id"]}))
            runtime = {"environment": {"PATH": str(Path(python).parent) + os.pathsep + os.environ["PATH"],
                         "PYTHONPATH": "src" + os.pathsep + os.environ.get("PYTHONPATH", "")},
                       "versions": [[python, "-m", "pytest", "--version"]], "timeout_seconds": 60}
            weak = "def test_existing(): assert True\n"
            for value, integrated in ((-1, True), (2, False)):
                (workspace / "src/value.py").write_text(f"def value(): return {value}\n")
                (workspace / "tests/test_value.py").write_text(weak)
                (workspace / "conftest.py").write_text("def pytest_collection_modifyitems(items): items.clear()\n")
                (workspace / "tests/test_added.py").write_text("raise RuntimeError('new test must stay in candidate only')\n")
                (agent / "result.json").write_text(json.dumps({"valid_integration": integrated,
                                                              "turns": [{"returncode": 0}]}))
                index = workspace / ".git/index"
                original_index, original_mtime = index.read_bytes(), index.stat().st_mtime_ns
                files = assess_candidate.working_files(workspace)
                output = root / ("failed-behavior" if value == -1 else "missing-hooks")
                result = assess_candidate.assess(task, source, workspace, agent, output, runtime)
                self.assertNotIn("error", result)
                self.assertTrue(result["correctness_evaluated"])
                expected = "fail" if value == -1 else "pass"
                self.assertEqual((result["existing"]["status"], result["hidden"]["status"]), (expected, expected))
                self.assertEqual(result["valid_correctness_sample"], integrated)
                self.assertEqual(result["hidden"]["test_result_events"], 3)
                if value == -1:
                    self.assertIn("test_hidden[<a&b>]", [failure["Test"] for failure in result["hidden"]["failures"]])
                self.assertEqual(assess_candidate.working_files(workspace), files)
                self.assertEqual((index.read_bytes(), index.stat().st_mtime_ns), (original_index, original_mtime))
                self.assertFalse((workspace / "tests/test_hidden.py").exists())
                self.assertEqual((output / "candidate/tests/test_value.py").read_text(), weak)
                self.assertTrue((output / "candidate/tests/test_added.py").exists())
                self.assertEqual((output / "validation/tests/test_value.py").read_text(), original)
                self.assertEqual((output / "validation/conftest.py").read_text(), fixture)
                self.assertFalse((output / "validation/tests/test_added.py").exists())
                self.assertFalse(list((output / "validation/src").glob("__pycache__/*.pyc")))

            (workspace / "pytest.ini").write_text("[pytest]\npython_functions=not_collected\n")
            changed = assess_candidate.assess(task, source, workspace, agent, root / "changed-configuration", runtime)
            self.assertEqual(changed["changed_test_configuration"], ["pytest.ini"])
            self.assertFalse(changed["correctness_evaluated"])
            self.assertNotIn("commands", changed["existing"])
            (workspace / "pytest.ini").unlink()
            (task / "hidden.patch").write_bytes((task / "reference.patch").read_bytes())
            manifest["sha256"]["hidden.patch"] = dataset.digest(task / "hidden.patch")
            (task / "task.json").write_text(json.dumps(manifest))
            invalid = assess_candidate.assess(task, source, workspace, agent, root / "invalid-hidden", runtime)
            self.assertIn("non-test files", invalid["error"])
            self.assertFalse(invalid["valid_correctness_sample"])
            self.assertEqual(assess_candidate.working_files(workspace), files)

    def test_native_report_must_be_complete_consistent_and_have_executed_tests(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / "pytest.xml"

            def check(xml, expected, code=0, timed_out=False):
                path.write_text(xml)
                record = {"test_report": path.name, "test_report_sha256": dataset.digest(path),
                          "exit_code": code, "timed_out": timed_out}
                result = assess_candidate.pytest_status([record], root)
                self.assertEqual(result["status"], expected)
                return result, record

            def suite(cases, tests=1, failures=0, errors=0, skipped=0):
                return (f'<testsuites><testsuite tests="{tests}" failures="{failures}" errors="{errors}" skipped="{skipped}">'
                        + cases + '</testsuite></testsuites>')

            success = suite('<testcase classname="test_value" name="test_ok"/>')
            failure = suite('<testcase classname="test_value" name="test_value"><failure message="bad">assert</failure></testcase>', failures=1)
            skip = suite('<testcase name="test_skip"><skipped message="optional"/></testcase>', skipped=1)
            check(success, "pass")
            check(success, "unknown", code=1)
            check(success, "unknown", timed_out=True)
            check(failure, "fail", code=1)
            check(failure, "unknown")
            check(failure, "unknown", code=2)
            self.assertEqual(len(check(skip, "unknown")[0]["skipped"]), 1)
            check(suite('', tests=0), "unknown")
            check(suite('<testcase name="test_ok"/>', tests=2), "unknown")
            check(suite('<testcase name="test_bad"><failure/><skipped/></testcase>', failures=1), "unknown")
            check("<incomplete", "unknown")
            _, record = check(success, "pass")
            path.write_text(failure)
            self.assertEqual(assess_candidate.pytest_status([record], root)["status"], "unknown")
            path.unlink()
            self.assertEqual(assess_candidate.pytest_status([record], root)["status"], "unknown")


if __name__ == "__main__":
    unittest.main()
