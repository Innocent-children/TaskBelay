import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

import collect_feedback
import collect_usage
import run_agents


class FeedbackTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.build.cleanup)
        configured = os.environ.get("TASKBELAY_EVAL_FEEDBACK_RENDERER")
        cls.renderer = Path(configured) if configured else Path(cls.build.name) / "feedback"
        if not configured:
            subprocess.run(["go", "build", "-o", str(cls.renderer), "./eval/cmd/feedback"],
                           cwd=Path(__file__).resolve().parents[2], capture_output=True, check=True,
                           env={**os.environ, "GOTOOLCHAIN": "go1.26.0"}, timeout=120)

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        self.metadata = {"agent": "claude", "workspace": str(self.root)}

    def hook(self, identity, kind="stop", **fields):
        decision = {key: "" for key in collect_feedback.DECISION_FIELDS}
        decision.update(fields)
        return {"id": identity, "agent": self.metadata["agent"], "session_id": "s", "path": str(self.root),
                "kind": kind, "state": "recorded", "error": "", "decision": json.dumps(decision)}

    def receipt(self, hooks):
        return {"turns": [{"turn": 1, "session_id": "s", "hooks": hooks, "events": {"session_id": "s"}}]}

    def test_both_adapters_measure_rendered_text_and_keep_estimates_separate(self):
        for agent in ("claude", "codex"):
            self.metadata["agent"] = agent
            hooks = [self.hook(1, AdditionalContext="not sent by Stop", UserMessage="中文😀abcd", BlockStop="stop"),
                     self.hook(2, "edit_use", AdditionalContext="read", DenyTool="deny")]
            result = collect_feedback.collect(self.metadata, self.receipt(hooks), self.renderer)
            self.assertEqual(result["status"], "recorded")
            self.assertEqual(result["observed"], {"hooks": 2, "hooks_with_text": 2, "fields": 4,
                                                  "utf8_bytes": 26, "characters": 19, "estimated_tokens": 7})
            self.assertIsNone(result["total_taskbelay_feedback"])

    def test_failed_pending_and_legacy_records_do_not_turn_into_zero_feedback(self):
        hooks = [self.hook(1), self.hook(2), self.hook(3), self.hook(4)]
        hooks[1]["state"] = "pending"
        hooks[2]["error"] = "output failed"
        hooks[3].pop("decision")
        result = collect_feedback.collect(self.metadata, self.receipt(hooks), self.renderer)
        self.assertEqual(result["status"], "partial")
        self.assertEqual(result["observed"]["hooks"], 1)
        self.assertEqual(result["observed"]["hooks_with_text"], 0)
        self.assertEqual([row["id"] for row in result["unknown"]], [2, 3, 4])

    def test_other_session_agent_workspace_and_repeated_ids_are_rejected(self):
        for key, value in (("agent", "codex"), ("session_id", "other"), ("path", str(self.root / "other")), ("id", True)):
            hook = self.hook(1)
            hook[key] = value
            result = collect_feedback.collect(self.metadata, self.receipt([hook]), self.renderer)
            self.assertEqual(result["status"], "unknown")
            self.assertIsNone(result["observed"])
        result = collect_feedback.collect(self.metadata, self.receipt([self.hook(1), self.hook(1)]), self.renderer)
        self.assertEqual(result["status"], "unknown")

    def test_invalid_decisions_or_adapter_mismatch_remain_unknown(self):
        for value in (None, [], {}, {key: 1 for key in collect_feedback.DECISION_FIELDS}):
            hook = self.hook(1)
            hook["decision"] = json.dumps(value)
            self.assertEqual(collect_feedback.collect(self.metadata, self.receipt([hook]), self.renderer)["status"], "unknown")
        for hook in (self.hook(1, "unsupported"), self.hook(1, DenyTool="invalid at Stop")):
            result = collect_feedback.collect(self.metadata, self.receipt([hook]), self.renderer)
            self.assertEqual(result["status"], "partial")
            self.assertEqual(result["observed"]["hooks"], 0)
        self.assertEqual(collect_feedback.collect(self.metadata, self.receipt([]), None)["status"], "unknown")

    def test_renderer_identity_and_counts_are_validated(self):
        receipt = self.receipt([self.hook(1)])
        for outputs in ([{"id": 2, "fields": [], "unknown": []}],
                        [{"id": 1, "fields": [{"name": "systemMessage", "utf8_bytes": True,
                                               "characters": 1, "estimated_tokens": 1}], "unknown": []}]):
            native = subprocess.CompletedProcess([], 0, json.dumps(outputs).encode(), b"")
            with mock.patch.object(collect_feedback.subprocess, "run", return_value=native):
                result = collect_feedback.collect(self.metadata, receipt, self.renderer)
            self.assertEqual(result["status"], "unknown")

    def test_usage_rechecks_evidence_after_feedback_measurement(self):
        path = self.root / "turn-1.jsonl"
        path.write_text(json.dumps({"type": "result", "subtype": "success", "session_id": "s", "usage": {}}) + "\n")
        receipt = self.receipt([self.hook(1)])
        receipt["turns"][0].update(duration_seconds=1, events=run_agents.parse_events(path, "claude"))
        for name, value in (("metadata.json", self.metadata), ("result.json", receipt)):
            (self.root / name).write_text(json.dumps(value))
        measured = collect_usage.collect(self.root, self.metadata, receipt, feedback_renderer=self.renderer)
        self.assertTrue(measured["valid_evidence"])
        self.assertEqual(measured["taskbelay_feedback"]["status"], "recorded")
        self.assertIsNone(measured["user_confirmations"])

        def mutate(*_args):
            (self.root / "result.json").write_text("changed")
            return {"status": "recorded"}

        with mock.patch.object(collect_feedback, "collect", side_effect=mutate):
            changed = collect_usage.collect(self.root, self.metadata, receipt, feedback_renderer=self.renderer)
        self.assertFalse(changed["valid_evidence"])


if __name__ == "__main__":
    unittest.main()
