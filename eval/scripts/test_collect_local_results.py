import json
from pathlib import Path
import tempfile
import unittest

import collect_local_results


class ResultCollectionTests(unittest.TestCase):
    def test_language_requires_separate_native_evidence_and_all_shared_rules(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            cases = root / "cases"
            cases.mkdir()
            regular, native = root / "regular.jsonl", root / "native.jsonl"
            coverage = {"workflows": [], "rules": [{"id": "T2", "tests": ["Rule"]}], "languages": [
                {"id": "python", "facts": [{"id": "assertions", "tests": ["Facts"]}],
                 "rule_ids": ["T2"], "native": {"id": "pytest", "tests": ["Native"]}}]}
            regular.write_text("\n".join(json.dumps({"Action": outcome, "Package": "eval", "Test": name})
                for name, outcome in [("Rule", "pass"), ("Facts", "pass"), ("Native", "skip")]), encoding="utf-8")
            report = collect_local_results.collect([regular], coverage, cases)
            self.assertEqual(report["languages"][0]["result"], "partial")
            self.assertEqual(report["languages"][0]["native"]["result"], "not_run")
            native.write_text(json.dumps({"Action": "pass", "Package": "eval", "Test": "Native"}), encoding="utf-8")
            report = collect_local_results.collect([regular], coverage, cases, [native])
            self.assertEqual(report["languages"][0]["result"], "pass")
            self.assertEqual(len(report["skipped_tests"]), 1)
            self.assertEqual(report["languages"][0]["native"]["outcomes"][0]["source"], str(native))
            coverage["rules"][0]["tests"].append("MissingBoundary")
            self.assertEqual(collect_local_results.collect([regular], coverage, cases, [native])["languages"][0]["result"], "partial")
            coverage["languages"][0]["rule_ids"].append("unknown")
            with self.assertRaisesRegex(ValueError, "unknown language rule"):
                collect_local_results.collect([regular], coverage, cases, [native])

    def test_first_issue_block_requires_exact_identity_and_cannot_repeat(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "test.jsonl"
            name = "TestWorkflowRecovery/WF-15"
            step = {"id": "WF-15", "event": 1, "decision": {"DenyTool": "", "BlockStop": "[D1 block] main.go:unused；编号=i1\n",
                    "AdditionalContext": "", "UserMessage": ""}, "allowed_block": "D1:i1",
                    "expected_quiet": False, "invalid_block": False, "extra_approval": False, "duplicate_prompt": False}
            summary = {"id": "WF-15", "events": 1, "invalid_blocks": 0, "extra_approvals": 0,
                       "duplicate_prompts": 0, "complete": True}

            def write(steps):
                summary["events"] = len(steps)
                output = "".join("TASKBELAY_WORKFLOW_EVENT " + json.dumps(s) + "\n" for s in steps)
                output += "TASKBELAY_WORKFLOW_METRICS " + json.dumps(summary) + "\n"
                log.write_text(json.dumps({"Action": "output", "Package": "eval", "Test": name, "Output": output})
                               + "\n" + json.dumps({"Action": "pass", "Package": "eval", "Test": name}), encoding="utf-8")

            write([step])
            self.assertEqual(collect_local_results.workflow_metrics([log])["WF-15"][0]["invalid_blocks"], 0)
            write([step, {**step, "event": 2}])
            with self.assertRaisesRegex(ValueError, "counts disagree"):
                collect_local_results.workflow_metrics([log])
            step["allowed_block"] = "D1:i2"
            write([step])
            with self.assertRaisesRegex(ValueError, "counts disagree"):
                collect_local_results.workflow_metrics([log])

    def test_workflow_counters_require_actual_complete_event_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            cases = root / "cases"
            cases.mkdir()
            log = root / "test.jsonl"
            name = "TestWorkflowInterruptions/WF-01"
            coverage = {"workflows": [{"id": "WF-01", "tests": [name], "metrics_test": name}]}
            step = {"id": "WF-01", "event": 1, "decision": {"DenyTool": "[S1] confirm", "BlockStop": "",
                    "AdditionalContext": "", "UserMessage": ""}, "allowed_block": "",
                    "expected_quiet": True, "invalid_block": True, "extra_approval": True,
                    "duplicate_prompt": True}
            summary = {"id": "WF-01", "events": 1, "invalid_blocks": 1, "extra_approvals": 1,
                       "duplicate_prompts": 1, "complete": True}

            def write(outcome="pass"):
                text = "TASKBELAY_WORKFLOW_EVENT " + json.dumps(step) + "\n"
                chunks = [text[:75], text[75:], "TASKBELAY_WORKFLOW_METRICS " + json.dumps(summary) + "\n"]
                events = [{"Action": "output", "Package": "eval", "Test": name, "Output": chunk} for chunk in chunks]
                events.append({"Action": outcome, "Package": "eval", "Test": name})
                log.write_text("\n".join(map(json.dumps, events)), encoding="utf-8")

            write()
            result = collect_local_results.collect([log], coverage, cases)["workflows"][0]
            self.assertEqual((result["invalid_blocks"], result["extra_approvals"], result["duplicate_prompts"]), (1, 1, 1))
            self.assertEqual(len(result["measurement_attempts"][0]["steps"]), 1)
            write("fail")
            self.assertIsNone(collect_local_results.collect([log], coverage, cases)["workflows"][0]["invalid_blocks"])
            summary["complete"] = False
            write()
            self.assertIsNone(collect_local_results.collect([log], coverage, cases)["workflows"][0]["extra_approvals"])
            summary["invalid_blocks"] = 0
            write()
            with self.assertRaisesRegex(ValueError, "incorrect workflow aggregate"):
                collect_local_results.collect([log], coverage, cases)
            summary["complete"], summary["invalid_blocks"] = True, 1
            step["allowed_block"] = "S1"
            step["decision"]["AdditionalContext"] = "[S2] another approval"
            write()
            result = collect_local_results.collect([log], coverage, cases)["workflows"][0]
            self.assertEqual(result["extra_approvals"], 1)
            self.assertEqual(result["invalid_blocks"], 1)

    def test_retry_and_skip_do_not_erase_missing_or_failed_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            cases = root / "cases"
            case = cases / "OD-01"
            (case / "base").mkdir(parents=True)
            for name in ["ledger.yaml", "expected.yaml", "README.md", "change.patch"]:
                (case / name).write_text("fixture", encoding="utf-8")
            log = root / "test.jsonl"
            events = [{"Action": result, "Package": "eval", "Test": test}
                      for test, result in [("TestCasePacks/OD-01", "fail"),
                                           ("TestCasePacks/OD-01", "pass"), ("Native", "skip"), ("Current", "pass")]]
            log.write_text("\n".join(map(json.dumps, events)), encoding="utf-8")
            report = collect_local_results.collect([log], {"rules": [
                {"id": "D1", "tests": ["TestCasePacks/OD-01", "missing"]},
                {"id": "D6", "tests": ["Native"], "gaps": ["exception"]},
                {"id": "D7", "tests": ["Current"], "gaps": ["exception"]}], "workflows": [
                {"id": "WF-01", "tests": ["missing"]}, {"id": "WF-02", "tests": ["Native"]}]}, cases)
            self.assertEqual(report["cases"][0]["result"], "fail")
            self.assertEqual(len(report["cases"][0]["outcomes"]), 2)
            self.assertEqual(report["workflows"][0]["result"], "not_run")
            self.assertEqual(report["workflows"][1]["result"], "skip")
            self.assertIsNone(report["workflows"][1]["extra_approvals"])
            self.assertEqual(report["counts"]["passed_case_packs"], 0)
            self.assertEqual(report["product_adoption"], "not_evaluated")
            self.assertEqual(report["rules"][0]["result"], "fail")
            self.assertEqual(report["rules"][0]["missing_tests"], ["missing"])
            self.assertEqual(report["rules"][1]["result"], "skip")
            self.assertEqual(report["rules"][2]["result"], "partial")


if __name__ == "__main__":
    unittest.main()
