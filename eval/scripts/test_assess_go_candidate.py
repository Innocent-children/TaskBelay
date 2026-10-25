import json
import os
from pathlib import Path
import shutil
import tempfile
import unittest

import assess_go_candidate
import dataset


class CandidateAssessmentTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which("go"), "native Go is required for candidate correctness")
    def test_original_assertions_and_hidden_tests_run_without_mutating_agent_workspace(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            source, task, agent = root / "source", root / "task", root / "agent"
            for path in (source, task, agent):
                path.mkdir()
            dataset.git(source, "init", "--initial-branch=main")

            def commit():
                dataset.git(source, "add", "--all")
                dataset.git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                            "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
                return dataset.git(source, "rev-parse", "HEAD").stdout.decode().strip()

            (source / "go.mod").write_text("module example.test/candidate\n\ngo 1.26\n")
            (source / "value.go").write_text("package candidate\nfunc Value()int{return 1}\n")
            original = 'package candidate\nimport "testing"\nfunc TestExisting(t *testing.T){if Value()<0{t.Fatal("negative")}}\n'
            (source / "value_test.go").write_text(original)
            base = commit()
            (source / "value.go").write_text("package candidate\nfunc Value()int{return 2}\n")
            (source / "hidden_test.go").write_text('package candidate\nimport "testing"\nfunc TestHidden(t *testing.T){if Value()!=2{t.Fatal("expected 2")}}\n')
            merged = commit()
            for name, paths in (("reference.patch", []), ("implementation.patch", ["value.go"]),
                                ("hidden.patch", ["hidden_test.go"])):
                (task / name).write_bytes(dataset.git(source, "diff", base, merged, "--", *paths).stdout)
            (task / "request.md").write_text("Return the requested value.\n")
            manifest = {"id": "fixture", "language": "Go", "base_commit": base, "merge_commit": merged,
                        "base_tree": dataset.git(source, "rev-parse", base + "^{tree}").stdout.decode().strip(),
                        "files": [{"path": "value.go", "category": "source"}],
                        "sha256": {path.name: dataset.digest(path) for path in task.iterdir()},
                        "checks": {"existing": [["go", "test", "./..."]], "hidden": [["go", "test", "./..."]]}}
            (task / "task.json").write_text(json.dumps(manifest))
            workspace = root / "workspace"
            dataset.prepare(task, source, workspace)
            (agent / "metadata.json").write_text(json.dumps({"workspace": str(workspace), "task_id": "fixture"}))
            runtime = {"environment": {"GOTOOLCHAIN": os.environ.get("GOTOOLCHAIN", "go1.26.0")},
                       "versions": [["go", "version"]], "timeout_seconds": 60}
            weak = 'package candidate\nimport "testing"\nfunc TestExisting(t *testing.T){}\n'
            for value, integrated in ((-1, True), (2, False)):
                (workspace / "value.go").write_text(f"package candidate\nfunc Value()int{{return {value}}}\n")
                (workspace / "value_test.go").write_text(weak)
                (workspace / "added_test.go").write_text('package candidate\nimport "testing"\nfunc TestAdded(t *testing.T){}\n')
                (agent / "result.json").write_text(json.dumps({"valid_integration": integrated,
                                                              "turns": [{"returncode": 0}]}))
                original_index = (workspace / ".git/index").read_bytes()
                files = assess_go_candidate.working_files(workspace)
                output = root / ("failed-behavior" if value == -1 else "missing-hooks")
                result = assess_go_candidate.assess(task, source, workspace, agent, output, runtime)
                self.assertNotIn("error", result)
                self.assertTrue(result["correctness_evaluated"])
                expected = "fail" if value == -1 else "pass"
                self.assertEqual((result["existing"]["status"], result["hidden"]["status"]), (expected, expected))
                self.assertEqual(result["valid_correctness_sample"], integrated)
                self.assertFalse(result["metrics_evaluated"])
                self.assertEqual(assess_go_candidate.working_files(workspace), files)
                self.assertEqual((workspace / ".git/index").read_bytes(), original_index)
                self.assertFalse((workspace / "hidden_test.go").exists())
                self.assertEqual((output / "candidate/value_test.go").read_text(), weak)
                self.assertTrue((output / "candidate/added_test.go").exists())
                self.assertEqual((output / "validation/value_test.go").read_text(), original)
                self.assertFalse((output / "validation/added_test.go").exists())
                self.assertTrue((output / "validation/hidden_test.go").exists())
            (task / "hidden.patch").write_bytes((task / "reference.patch").read_bytes())
            manifest["sha256"]["hidden.patch"] = dataset.digest(task / "hidden.patch")
            (task / "task.json").write_text(json.dumps(manifest))
            invalid = assess_go_candidate.assess(task, source, workspace, agent, root / "invalid-hidden", runtime)
            self.assertIn("non-test files", invalid["error"])
            self.assertEqual(invalid["hidden"]["status"], "unknown")
            self.assertFalse(invalid["correctness_evaluated"])
            self.assertEqual(assess_go_candidate.working_files(workspace), files)

    def test_skips_incomplete_and_contradictory_output_do_not_count_as_passes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            result = {"stdout": "go.jsonl", "exit_code": 0, "timed_out": False}

            def check(events, outcome, record=result):
                (root / "go.jsonl").write_text("".join(json.dumps(event) + "\n" for event in events))
                report = assess_go_candidate.test_status([record], root)
                self.assertEqual(report["status"], outcome)
                return report

            skip = {"Action": "skip", "Package": "fixture", "Test": "TestSkipped"}
            success = {"Action": "pass", "Package": "fixture", "Test": "TestExecuted"}
            failure = {"Action": "fail", "Package": "fixture", "Test": "TestFailed"}
            self.assertEqual(check([skip], "unknown")["skipped"], [skip])
            check([success, skip], "pass")
            check([success], "unknown", {**result, "timed_out": True})
            check([failure], "unknown")
            check([failure], "fail", {**result, "exit_code": 1})
            check([], "unknown")


if __name__ == "__main__":
    unittest.main()
