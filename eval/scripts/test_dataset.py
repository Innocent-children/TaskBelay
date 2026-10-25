import json
import os
from pathlib import Path
import tempfile
import unittest

import dataset


class DatasetPreparationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.source = self.root / "source"
        self.source.mkdir()
        dataset.git(self.source, "init", "--initial-branch=main")
        (self.source / "base.txt").write_text("original\n")
        (self.source / ".gitattributes").write_text("base.txt export-ignore\n")
        self.base = self.commit("base")
        (self.source / "base.txt").write_text("reference answer\n")
        (self.source / "hidden.txt").write_text("secret acceptance\n")
        self.reference = self.commit("reference")
        self.task = self.root / "task"
        self.task.mkdir()
        reference = dataset.git(self.source, "diff", self.base, self.reference).stdout
        (self.task / "reference.patch").write_bytes(reference)
        (self.task / "implementation.patch").write_bytes(reference)
        (self.task / "hidden.patch").write_bytes(reference)
        (self.task / "request.md").write_text("Change the observable behavior.\n")
        self.metadata = {"id": "test", "base_commit": self.base, "merge_commit": self.reference,
            "base_tree": dataset.git(self.source, "rev-parse", self.base + "^{tree}").stdout.decode().strip(),
            "sha256": {path.name: dataset.digest(path) for path in self.task.iterdir()}}
        self.write_metadata()

    def write_metadata(self):
        (self.task / "task.json").write_text(json.dumps(self.metadata))

    def commit(self, message):
        dataset.git(self.source, "add", "--all")
        dataset.git(self.source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                    "-c", "commit.gpgsign=false", "commit", "-m", message)
        return dataset.git(self.source, "rev-parse", "HEAD").stdout.decode().strip()

    def test_exact_base_tree_without_future_history_or_export_filtering(self):
        workspace = self.root / "工作区 space"
        receipt = dataset.prepare(self.task, self.source, workspace)
        self.assertFalse(receipt["hidden_tests_added"])
        self.assertEqual((workspace / "base.txt").read_text(), "original\n")
        self.assertFalse((workspace / "hidden.txt").exists())
        self.assertEqual(dataset.git(workspace, "rev-list", "--count", "HEAD").stdout, b"1\n")
        self.assertEqual(dataset.git(workspace, "remote").stdout, b"")
        self.assertEqual(dataset.git(workspace, "status", "--porcelain").stdout, b"")
        self.assertNotEqual(dataset.git(workspace, "cat-file", "-e", self.reference, check=False).returncode, 0)
        answer_blob = dataset.git(self.source, "rev-parse", self.reference + ":hidden.txt").stdout.decode().strip()
        self.assertNotEqual(dataset.git(workspace, "cat-file", "-e", answer_blob, check=False).returncode, 0)

    def test_changed_evidence_wrong_parent_and_existing_work_are_rejected(self):
        workspace = self.root / "new"
        (self.task / "reference.patch").write_text("tampered")
        with self.assertRaises(ValueError):
            dataset.prepare(self.task, self.source, workspace)
        self.assertFalse(workspace.exists())
        self.metadata["sha256"] = {}
        self.metadata["base_commit"] = self.reference
        self.metadata["base_tree"] = dataset.git(self.source, "rev-parse", self.reference + "^{tree}").stdout.decode().strip()
        self.write_metadata()
        with self.assertRaises(ValueError):
            dataset.prepare(self.task, self.source, workspace)
        with self.assertRaises(ValueError):
            dataset.prepare(self.task, self.source, self.source)
        self.assertEqual((self.source / "hidden.txt").read_text(), "secret acceptance\n")

    @unittest.skipIf(os.name == "nt", "symlink privileges need native Windows verification")
    def test_outside_symlink_is_rejected_before_materializing_files(self):
        (self.source / "outside").symlink_to("../../outside")
        base = self.commit("link")
        (self.source / "change.txt").write_text("next")
        merged = self.commit("next")
        self.metadata.update(base_commit=base, merge_commit=merged,
            base_tree=dataset.git(self.source, "rev-parse", base + "^{tree}").stdout.decode().strip())
        self.write_metadata()
        workspace = self.root / "new"
        with self.assertRaises(ValueError):
            dataset.prepare(self.task, self.source, workspace)
        self.assertFalse(workspace.exists())


if __name__ == "__main__":
    unittest.main()
