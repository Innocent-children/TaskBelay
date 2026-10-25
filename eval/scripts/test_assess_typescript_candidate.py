import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

import assess_candidate
import dataset


class TypeScriptCandidateAssessmentTests(unittest.TestCase):
    def test_native_vitest_keeps_original_runtime_type_and_setup_assertions(self):
        tools = Path(os.environ.get("TASKBELAY_EVAL_JS_TOOLS", "/missing"))
        node = shutil.which("node")
        if not node or not (tools / "vitest/vitest.mjs").is_file() or not (tools / "typescript/bin/tsc").is_file():
            self.skipTest("native Vitest and TypeScript tools are required")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            source, task, agent = (root / name for name in ("source", "task", "agent"))
            for path in (source, task, agent):
                path.mkdir()
            dataset.git(source, "init", "--initial-branch=main")
            (source / "library").mkdir()
            (source / ".gitignore").write_text("node_modules/\n")
            (source / "package.json").write_text('{"private":true,"type":"module"}\n')
            (source / "library/vitest.config.mts").write_text(
                "import { defineConfig } from 'vitest/config'; export default defineConfig({test:{"
                "setupFiles:['./setup.ts'],typecheck:{enabled:true,tsconfig:'./tsconfig.json'}}});\n")
            (source / "library/tsconfig.json").write_text(json.dumps({"compilerOptions": {
                "module": "ESNext", "moduleResolution": "bundler", "skipLibCheck": True, "strict": True,
                "target": "ESNext"}, "include": ["*.ts"]}))
            (source / "library/value.ts").write_text("export const value = () => 1;\n")
            original = "import {test,expect} from 'vitest'; import {value} from './value'; test('existing',()=>expect(value()).toBeGreaterThan(0));\n"
            types = "import {test,expectTypeOf} from 'vitest'; import {value} from './value'; test('type contract',()=>expectTypeOf(value).returns.toEqualTypeOf<number>());\n"
            (source / "library/value.test.ts").write_text(original)
            (source / "library/value.test-d.ts").write_text(types)
            (source / "library/setup.ts").write_text("export {};\n")

            def commit():
                dataset.git(source, "add", "--all")
                dataset.git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                            "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
                return dataset.git(source, "rev-parse", "HEAD").stdout.decode().strip()

            base = commit()
            (source / "library/value.ts").write_text("export const value = () => 2;\n")
            (source / "library/hidden.test.ts").write_text(
                "import {test,expect} from 'vitest'; import {value} from './value'; test.each([2,2])('hidden %s 中文',v=>expect(value()).toBe(v));\n")
            merged = commit()
            for name, paths in (("reference.patch", []), ("implementation.patch", ["library/value.ts"]),
                                ("hidden.patch", ["library/hidden.test.ts"])):
                (task / name).write_bytes(dataset.git(source, "diff", base, merged, "--", *paths).stdout)
            (task / "request.md").write_text("Return the requested value.\n")
            # Resolve pnpm package links before creating an isolated .bin entry.
            prepare = ("from pathlib import Path; p=Path('library/node_modules'); (p/'.bin').mkdir(parents=True); "
                       + f"(p/'vitest').symlink_to({str((tools/'vitest').resolve())!r},target_is_directory=True); "
                       + f"(p/'typescript').symlink_to({str((tools/'typescript').resolve())!r},target_is_directory=True); "
                       + f"(p/'.bin/tsc').symlink_to({str((tools/'typescript/bin/tsc').resolve())!r})")
            checks = {phase: [{"cwd": "library", "argv": [node, "node_modules/vitest/vitest.mjs", "run",
                                                          "--typecheck", "--maxWorkers", "1"]}]
                      for phase in ("existing", "hidden")}
            checks["prepare"] = [[sys.executable, "-c", prepare]]
            manifest = {"id": "typescript-fixture", "language": "TypeScript", "base_commit": base, "merge_commit": merged,
                        "base_tree": dataset.git(source, "rev-parse", base + "^{tree}").stdout.decode().strip(),
                        "files": [{"path": "library/value.ts", "category": "source"}],
                        "sha256": {p.name: dataset.digest(p) for p in task.iterdir()}, "checks": checks}
            (task / "task.json").write_text(json.dumps(manifest))
            workspace = root / "workspace"
            dataset.prepare(task, source, workspace)
            (agent / "metadata.json").write_text(json.dumps({"workspace": str(workspace), "task_id": manifest["id"]}))
            runtime = {"test_support_files": ["library/setup.ts"], "timeout_seconds": 60,
                       "versions": [[node, "--version"]]}
            for mode, implementation, expected in (
                    ("behavior", "export const value = () => -1;\n", "fail"),
                    ("type", "export const value = (): string => 2 as unknown as string;\n", "fail"),
                    ("missing-hooks", "export const value = () => 2;\n", "pass")):
                (workspace / "library/value.ts").write_text(implementation)
                (workspace / "library/value.test.ts").write_text("import {test} from 'vitest'; test('weakened',()=>{});\n")
                (workspace / "library/value.test-d.ts").write_text("import {test} from 'vitest'; test('weakened type',()=>{});\n")
                (workspace / "library/setup.ts").write_text("throw new Error('candidate setup');\n")
                (workspace / "library/added.test.ts").write_text("throw new Error('candidate-only test');\n")
                (agent / "result.json").write_text(json.dumps({"valid_integration": mode != "missing-hooks", "turns": [{"returncode": 0}]}))
                index = workspace / ".git/index"
                before = (assess_candidate.working_files(workspace), index.read_bytes(), index.stat().st_mtime_ns)
                result = assess_candidate.assess(task, source, workspace, agent, root / mode, runtime)
                self.assertNotIn("error", result)
                self.assertTrue(result["correctness_evaluated"], result)
                self.assertEqual((result["existing"]["status"], result["hidden"]["status"]), (expected, expected))
                self.assertEqual(result["valid_correctness_sample"], mode != "missing-hooks")
                self.assertEqual(result["hidden"]["test_result_events"], 4)
                if mode == "type":
                    self.assertTrue(any(f.get("typecheck") for f in result["existing"]["failures"]))
                self.assertEqual((assess_candidate.working_files(workspace), index.read_bytes(), index.stat().st_mtime_ns), before)
                self.assertFalse((workspace / "library/hidden.test.ts").exists())
                self.assertTrue((root / mode / "candidate/library/added.test.ts").exists())
                self.assertFalse((root / mode / "validation/library/added.test.ts").exists())
                self.assertEqual((root / mode / "validation/library/value.test.ts").read_text(), original)
                self.assertEqual((root / mode / "validation/library/value.test-d.ts").read_text(), types)
                self.assertEqual((root / mode / "validation/library/setup.ts").read_text(), "export {};\n")
            missing = assess_candidate.assess(task, source, workspace, agent, root / "undeclared-setup",
                                             {"timeout_seconds": 60})
            self.assertIn("test_support_files", missing["error"])
            self.assertFalse(missing["correctness_evaluated"])
            (workspace / "library/vitest.config.mts").write_text("export default {test:{include:[]}};\n")
            result = assess_candidate.assess(task, source, workspace, agent, root / "changed-config", runtime)
            self.assertEqual(result["changed_test_configuration"], ["library/vitest.config.mts"])
            self.assertFalse(result["correctness_evaluated"])

    def test_report_counts_exit_and_hash_must_agree(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            report = {"numTotalTestSuites": 1, "numPassedTestSuites": 1, "numFailedTestSuites": 0,
                      "numPendingTestSuites": 0, "numTotalTests": 1, "numPassedTests": 1, "numFailedTests": 0,
                      "numPendingTests": 0, "numTodoTests": 0, "success": True,
                      "testResults": [{"name": str(root / "value.test.ts"), "status": "passed", "message": "",
                                       "assertionResults": [{"fullName": "value 中文", "status": "passed", "failureMessages": []}]}]}
            path = root / "native.json"

            def check(value, expected, code=0, timeout=False):
                path.write_text(json.dumps(value))
                record = {"test_report": path.name, "test_report_sha256": dataset.digest(path), "test_root": str(root),
                          "exit_code": code, "timed_out": timeout}
                result = assess_candidate.vitest_status([record], root)
                self.assertEqual(result["status"], expected, result)
                return record

            check(report, "pass")
            check(report, "unknown", code=1)
            check(report, "unknown", timeout=True)
            unnamed = copy.deepcopy(report); unnamed["testResults"][0]["assertionResults"][0]["fullName"] = ""
            check(unnamed, "pass")
            wrong = copy.deepcopy(report); wrong["numTotalTests"] = 2
            check(wrong, "unknown")
            empty = copy.deepcopy(report); empty["numPassedTests"] = 0; empty["numPendingTests"] = 1
            empty["testResults"][0]["assertionResults"][0]["status"] = "skipped"
            check(empty, "unknown")
            pending = copy.deepcopy(empty); pending["testResults"][0]["assertionResults"][0]["status"] = "pending"
            check(pending, "unknown")
            failure = copy.deepcopy(report)
            failure.update(numPassedTestSuites=0, numFailedTestSuites=1, numPassedTests=0, numFailedTests=1, success=False)
            failure["testResults"][0]["status"] = "failed"
            failure["testResults"][0]["assertionResults"][0].update(status="failed", failureMessages=["assertion failed"])
            check(failure, "fail", code=1)
            unnamed = copy.deepcopy(failure); unnamed["testResults"][0]["assertionResults"][0]["fullName"] = ""
            record = check(unnamed, "fail", code=1)
            evidence = assess_candidate.vitest_status([record], root)["failures"][0]
            self.assertEqual((evidence["Package"], evidence["Test"], evidence["case_index"]), ("value.test.ts", "", 0))
            check(failure, "unknown")
            check(failure, "unknown", code=2)
            load = copy.deepcopy(failure); load.update(numFailedTests=0, numTotalTests=0)
            load["testResults"][0].update(assertionResults=[], message="module failed to load")
            check(load, "fail", code=1)
            record = check(report, "pass")
            path.write_text(json.dumps(failure))
            self.assertEqual(assess_candidate.vitest_status([record], root)["status"], "unknown")
            path.unlink()
            self.assertEqual(assess_candidate.vitest_status([record], root)["status"], "unknown")


if __name__ == "__main__":
    unittest.main()
