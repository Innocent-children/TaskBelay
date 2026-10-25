import json
from pathlib import Path
import sys
import tempfile
import unittest

import dataset
import validate_dataset


class DatasetValidationTests(unittest.TestCase):
    def test_prepare_mutating_input_stops_before_tests_but_generated_output_is_allowed(self):
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

            (source / "app.txt").write_text("base")
            base = commit()
            (source / "app.txt").write_text("reference")
            (source / "hidden.txt").write_text("acceptance")
            merged = commit()
            for name, paths in (("reference.patch", []), ("implementation.patch", ["app.txt"]),
                                ("hidden.patch", ["hidden.txt"])):
                (task / name).write_bytes(dataset.git(source, "diff", base, merged, "--", *paths).stdout)
            (task / "request.md").write_text("Change the required behavior.\n")
            check = [sys.executable, "-c", "from pathlib import Path; import sys; "
                     "print('FAIL required_behavior'); sys.exit(Path('app.txt').read_text() != 'reference')"]
            metadata = {"id": "fixture", "base_commit": base, "merge_commit": merged,
                        "base_tree": dataset.git(source, "rev-parse", base + "^{tree}").stdout.decode().strip(),
                        "files": [{"path": "app.txt", "category": "source"}],
                        "sha256": {p.name: dataset.digest(p) for p in task.iterdir()},
                        "checks": {"existing": [[sys.executable, "-c", "print('existing passed')"]],
                                   "hidden": [check], "baseline_failure_patterns": ["FAIL required_behavior"]}}
            runtime = {"versions": [[sys.executable, "--version"]], "timeout_seconds": 30}
            for name, path in (("changed-input", "app.txt"), ("build-output", "generated.txt")):
                metadata["checks"]["prepare"] = [[sys.executable, "-c",
                    f"from pathlib import Path; Path({path!r}).write_text('generated')"]]
                (task / "task.json").write_text(json.dumps(metadata))
                report = validate_dataset.validate(task, source, root / name, runtime)
                self.assertNotIn("error", report)
                if name == "changed-input":
                    self.assertFalse(report["validated"])
                    for variant in report["variants"].values():
                        self.assertTrue(variant["preparation_failed"])
                        self.assertEqual(variant["preparation_changed_inputs"], ["app.txt"])
                        self.assertNotIn("existing", variant)
                        self.assertNotIn("hidden", variant)
                else:
                    self.assertTrue(report["validated"])
                self.assertEqual((source / "app.txt").read_text(), "reference")

    def test_timeout_missing_evidence_and_unrelated_failures_remain_unknown(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "out").write_text("FAIL TestRequiredBehavior: observed old behavior\n")
            (root / "err").write_text("")
            result = {"exit_code": 1, "timed_out": False, "stdout": "out", "stderr": "err"}
            checks = {"baseline_failure_patterns": ["FAIL TestRequiredBehavior"]}
            self.assertEqual(validate_dataset.baseline_outcome([result], root, checks), "fail")
            self.assertEqual(validate_dataset.baseline_outcome([result], root, {}), "unknown")
            self.assertEqual(validate_dataset.baseline_outcome([{**result, "timed_out": True}], root, checks), "unknown")
            self.assertEqual(validate_dataset.baseline_outcome([{**result, "exit_code": 2}], root, checks), "unknown")
            (root / "out").write_text("network unavailable\n")
            self.assertEqual(validate_dataset.baseline_outcome([result], root, checks), "unknown")
            self.assertEqual(validate_dataset.baseline_outcome([{**result, "exit_code": 0}], root, checks), "pass")
            self.assertEqual(validate_dataset.baseline_outcome([], root, checks), "unknown")


if __name__ == "__main__":
    unittest.main()
