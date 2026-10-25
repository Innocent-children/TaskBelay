import json
import contextlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import assess_candidate
import dataset


class RustCandidateCommandsTests(unittest.TestCase):
    def test_fixed_toolchain_features_and_working_directory_are_preserved(self):
        command = {"cwd": "crate", "argv": ["cargo", "+1.97.0", "test", "--locked", "--features", "raw_value,preserve_order"]}
        result = assess_candidate.test_commands({"language": "Rust", "checks": {"existing": [command], "hidden": [command]}}, Path("/evidence"))
        for phase in result.values():
            self.assertEqual(phase[0]["cwd"], "crate")
            self.assertEqual(phase[0]["argv"], command["argv"] + ["--message-format=json", "--", "--format=pretty", "--color=never"])
        for argument in ("--message-format=json", "--no-run", "--", "--quiet"):
            with self.subTest(argument=argument), self.assertRaises(ValueError):
                assess_candidate.test_commands({"language": "Rust", "checks": {
                    "existing": [["cargo", "test", argument]], "hidden": [["cargo", "test"]]}}, Path("/evidence"))


@unittest.skipUnless(os.environ.get("TASKBELAY_EVAL_RUST_NATIVE") == "1" and os.environ.get("TASKBELAY_EVAL_RUST_INPUTS")
                     and shutil.which("cargo"), "native Cargo and Rust syntax helper are not configured")
class RustCandidateNativeTests(unittest.TestCase):
    def test_native_candidate_restoration_docs_and_compile_time_hidden_acceptance(self):
        evidence = os.environ.get("TASKBELAY_EVAL_RUST_EVIDENCE")
        temporary_root = (contextlib.nullcontext(tempfile.mkdtemp(prefix="native-candidate-", dir=evidence))
                          if evidence else tempfile.TemporaryDirectory())
        with temporary_root as temporary:
            root = Path(temporary).resolve()
            source, task, agent = (root / name for name in ("source", "task", "agent"))
            for path in (source, task, agent):
                path.mkdir()
            dataset.git(source, "init", "--initial-branch=main")

            def commit():
                dataset.git(source, "add", "--all")
                dataset.git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                            "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
                return dataset.git(source, "rev-parse", "HEAD").stdout.decode().strip()

            (source / "src").mkdir()
            (source / "tests").mkdir()
            (source / ".gitignore").write_text("/target/\n")
            (source / "Cargo.toml").write_text('[package]\nname="fixture"\nversion="0.1.0"\nedition="2024"\n')
            original = '''/// ```
/// assert!(fixture::value() >= 0);
/// ```
pub fn value() -> i32 { 1 }
#[cfg(test)] mod checks {
    fn threshold() -> i32 { include_str!("threshold.txt").trim().parse().unwrap() }
    #[test] fn existing() { assert!(super::value() >= threshold()); }
    #[test] #[ignore="native fixture"] fn ignored() {}
}
'''
            (source / "src/lib.rs").write_text(original)
            (source / "src/threshold.txt").write_text("0\n")
            locked = subprocess.run(["cargo", "generate-lockfile", "--offline"], cwd=source, capture_output=True)
            self.assertEqual(locked.returncode, 0, locked.stderr)
            base = commit()
            addition = '''/// ```
/// assert_eq!(fixture::requested(), 2);
/// ```
pub fn requested() -> i32 { 2 }
'''
            (source / "src/lib.rs").write_text(original + addition)
            (source / "tests/hidden.rs").write_text("#[test] fn requested() { assert_eq!(fixture::requested(), 2); }\n")
            merged = commit()
            for name, paths in (("reference.patch", []), ("implementation.patch", ["src/lib.rs"]), ("hidden.patch", ["tests/hidden.rs"])):
                (task / name).write_bytes(dataset.git(source, "diff", base, merged, "--", *paths).stdout)
            (task / "request.md").write_text("Expose the requested behavior.\n")
            manifest = {"id": "rust-fixture", "language": "Rust", "base_commit": base, "merge_commit": merged,
                        "base_tree": dataset.git(source, "rev-parse", base + "^{tree}").stdout.decode().strip(),
                        "files": [{"path": "src/lib.rs", "category": "source"}],
                        "sha256": {path.name: dataset.digest(path) for path in task.iterdir()},
                        "checks": {"existing": [["cargo", "test", "--offline"]], "hidden": [["cargo", "test", "--offline"]],
                                   "baseline_failure_patterns": ["cannot find function `requested`"]}}
            (task / "task.json").write_text(json.dumps(manifest))
            workspace = root / "workspace"
            dataset.prepare(task, source, workspace)
            (agent / "metadata.json").write_text(json.dumps({"workspace": str(workspace), "task_id": manifest["id"]}))
            runtime = {"environment": {"CARGO_BUILD_JOBS": "2"}, "versions": [["rustc", "--version"], ["cargo", "--version"]],
                       "rust_test_inputs": os.environ["TASKBELAY_EVAL_RUST_INPUTS"], "timeout_seconds": 120}
            for variant, value, extra, expected in (("base", 1, "", ("pass", "fail")),
                ("broken", -1, addition, ("fail", "fail")), ("reference", 2, addition, ("pass", "pass")),
                ("added-doc-fails", 2, addition.replace("requested(), 2", "requested(), 999"), ("unknown", "unknown"))):
                with self.subTest(variant=variant):
                    text = original.replace("{ 1 }", f"{{ {value} }}").replace("assert!(super::value() >= threshold());", "") + extra
                    (workspace / "src/lib.rs").write_text(text)
                    (workspace / "src/threshold.txt").write_text("-999\n")
                    (workspace / "tests").mkdir(exist_ok=True)
                    (workspace / "tests/added.rs").write_text("not valid Rust and must not enter the validation copy")
                    integrated = variant != "reference"
                    (agent / "result.json").write_text(json.dumps({"valid_integration": integrated, "turns": [{"returncode": 0}]}))
                    files = assess_candidate.working_files(workspace)
                    index = workspace / ".git/index"
                    before_index = (index.read_bytes(), index.stat().st_mtime_ns)
                    output = root / variant
                    result = assess_candidate.assess(task, source, workspace, agent, output, runtime)
                    self.assertNotIn("error", result, result.get("error"))
                    self.assertEqual((result["existing"]["status"], result["hidden"]["status"]), expected, result)
                    self.assertEqual(result["correctness_evaluated"], "unknown" not in expected)
                    self.assertEqual(result["valid_correctness_sample"], integrated and "unknown" not in expected)
                    self.assertEqual(files, assess_candidate.working_files(workspace))
                    self.assertEqual((index.read_bytes(), index.stat().st_mtime_ns), before_index)
                    self.assertEqual((output / "candidate/src/lib.rs").read_text(), text)
                    restored = (output / "validation/src/lib.rs").read_text()
                    self.assertIn(f"pub fn value() -> i32 {{ {value} }}", restored)
                    self.assertIn("assert!(super::value() >= threshold());", restored)
                    self.assertEqual((output / "validation/src/threshold.txt").read_text(), "0\n")
                    self.assertFalse((output / "validation/tests/added.rs").exists())
                    self.assertFalse((workspace / "tests/hidden.rs").exists())
                    if variant == "base":
                        self.assertTrue(result["hidden"]["compiler_errors"])
                        self.assertEqual(result["hidden"]["failures"][0]["kind"], "compile")
                        self.assertEqual(result["hidden"]["failures"][0]["source"], "tests/hidden.rs")
                    if variant == "reference":
                        self.assertEqual(result["existing"]["test_result_events"], 2)
                        self.assertEqual(len(result["existing"]["additional_doc_cases"]), 1)
                        self.assertEqual(len(result["existing"]["skipped"]), 1)
                    for phase in ("existing", "hidden"):
                        context = assess_candidate.rust_context(result, output, manifest, phase)
                        checked = assess_candidate.test_status(result[phase]["commands"], output / phase, "Rust", context)
                        self.assertTrue(all(result[phase][key] == item for key, item in checked.items()))
            (workspace / "Cargo.toml").write_text((workspace / "Cargo.toml").read_text() + "\n[lib]\ndoctest=false\n")
            result = assess_candidate.assess(task, source, workspace, agent, root / "config", runtime)
            self.assertEqual(result["changed_test_configuration"], ["Cargo.toml"])
            self.assertFalse(result["correctness_evaluated"])


if __name__ == "__main__":
    unittest.main()
