import json
from pathlib import Path
import shutil
import tempfile
import unittest

import dataset
import reassess_dataset


class ReassessmentTests(unittest.TestCase):
    def test_only_classification_changes_can_reuse_intact_native_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            task = root / "task"
            task.mkdir()
            for name in ("request.md", "reference.patch", "implementation.patch", "hidden.patch"):
                (task / name).write_text("fixed input\n")
            metadata = {"id": "fixture", "base_commit": "a" * 40, "base_tree": "b" * 40,
                        "merge_commit": "c" * 40, "checks": {},
                        "sha256": {p.name: dataset.digest(p) for p in task.iterdir()}}
            (task / "task.json").write_text(json.dumps(metadata))
            output = root / "native"
            output.mkdir()
            shutil.copytree(task, output / "task-material")

            def command(path, code):
                path.mkdir(parents=True)
                (path / "out").write_text("FAILED required_behavior\n" if code else "passed\n")
                (path / "err").write_text("")
                return [{"exit_code": code, "timed_out": False, "stdout": "out", "stderr": "err",
                         "stdout_sha256": dataset.digest(path / "out"), "stderr_sha256": dataset.digest(path / "err")}]

            report = {"task_manifest_sha256": dataset.digest(task / "task.json"),
                      "versions": command(output / "versions", 0), "variants": {}}
            for variant in ("base", "reference"):
                report["variants"][variant] = {
                    phase: command(output / variant / phase, 1 if variant == "base" and phase == "hidden" else 0)
                    for phase in ("versions", "existing", "hidden")}
            (output / "result.json").write_text(json.dumps(report))
            metadata["checks"]["baseline_failure_patterns"] = ["FAILED required_behavior"]
            (task / "task.json").write_text(json.dumps(metadata))
            result = reassess_dataset.reassess(task, output / "result.json", root / "assessment.json")
            self.assertTrue(result["validated"])
            self.assertEqual(result["baseline_hidden_outcome"], "fail")
            adjacent = reassess_dataset.reassess(task, output / "result.json", output / "assessment.json")
            self.assertEqual(adjacent["source_result"], "result.json")
            self.assertEqual(adjacent["source_result_sha256"], dataset.digest(output / "result.json"))
            (task / "request.md").write_text("changed request\n")
            metadata["sha256"]["request.md"] = dataset.digest(task / "request.md")
            (task / "task.json").write_text(json.dumps(metadata))
            with self.assertRaisesRegex(ValueError, "execution input changed"):
                reassess_dataset.reassess(task, output / "result.json", root / "wrong.json")
            (task / "request.md").write_text("fixed input\n")
            metadata["sha256"]["request.md"] = dataset.digest(task / "request.md")
            (task / "task.json").write_text(json.dumps(metadata))
            (output / "base" / "hidden" / "out").write_text("changed evidence\n")
            with self.assertRaisesRegex(ValueError, "command output changed"):
                reassess_dataset.reassess(task, output / "result.json", root / "tampered.json")


if __name__ == "__main__":
    unittest.main()
