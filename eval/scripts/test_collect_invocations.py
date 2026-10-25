import copy
import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

import collect_invocations
import collect_usage
import dataset
import run_agents
import trace_command


class InvocationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.build.cleanup)
        configured = os.environ.get("TASKBELAY_EVAL_COMMAND_CLASSIFIER")
        cls.classifier = Path(configured) if configured else Path(cls.build.name) / "commands"
        if not configured:
            result = subprocess.run(["go", "build", "-o", str(cls.classifier), "./eval/cmd/commands"],
                cwd=Path(__file__).resolve().parents[2], capture_output=True, text=True,
                env={**os.environ, "GOTOOLCHAIN": "go1.26.0"}, timeout=120)
            if result.returncode:
                raise RuntimeError(result.stdout + result.stderr)

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)

    def command(self, identity, argv, parent=None, role="native", origin="agent", state="complete", code=0):
        attempt = {"event": "attempt", "id": identity, "parent_id": parent, "role": role,
                   "origin": origin, "argv": argv, "cwd": str(self.root)}
        if state == "not_started":
            return [attempt, {"event": "launch_failed", "id": identity, "error": "fixture missing tool"}]
        events = [attempt, {"event": "start", "id": identity, "pid": 123}]
        if state == "complete":
            events.append({"event": "exit", "id": identity, "returncode": code, "elapsed_seconds": 1})
        return events

    def invocation(self, identity, text, **kwargs):
        script = "source /snapshot 2>/dev/null || true; eval " + shlex.quote(text) + " < /dev/null"
        return self.command(identity, ["/bin/zsh", "-c", script], role="shell_invocation", **kwargs)

    def receipt(self, commands, events, denied=()):
        native = [{"type": "assistant", "message": {"content": [
            {"type": "tool_use", "name": "Bash", "id": identity, "input": {"command": text}}
            for identity, text in commands]}}]
        native.append({"type": "result", "subtype": "success", "permission_denials": [
            {"tool_name": "Bash", "tool_use_id": identity} for identity in denied], "usage": {}})
        path = self.root / "turn-1.jsonl"
        path.write_text("".join(json.dumps(event) + "\n" for event in native))
        trace = self.root / "command-trace.jsonl"
        trace.write_text("".join(json.dumps(event) + "\n" for event in events))
        receipt = {"turns": [{"turn": 1, "duration_seconds": 1, "events": run_agents.parse_events(path, "claude")}],
                   "command_trace": {"file": trace.name, "sha256": dataset.digest(trace),
                                     "records": trace_command.collect(trace)}}
        (self.root / "result.json").write_text(json.dumps(receipt))
        return receipt

    def collect(self, receipt):
        return collect_invocations.collect(self.root, "claude", receipt, self.classifier)

    def test_compound_call_counts_once_and_short_circuit_and_non_agent_do_not_count(self):
        compound, short = "go test ./... && go test ./...", "false && go test ./..."
        literal = "printf '%s' 'literal $HOME `text` 中文'"
        events = [*self.invocation("one", compound),
                  *self.command("a", ["/native/go", "test", "./..."], "one"),
                  *self.command("b", ["/native/go", "test", "./..."], "one"),
                  *self.invocation("two", short, code=1), *self.invocation("three", literal),
                  *self.command("product", ["/native/go", "test", "./..."], origin="taskbelay"),
                  *self.command("check", ["/native/go", "test", "./..."], origin="assessment")]
        receipt = self.receipt([("tool1", compound), ("tool2", short), ("tool3", literal)], events)
        result = self.collect(receipt)
        self.assertEqual(result["observed_verification_invocations"], 1, result)
        self.assertTrue(result["association_complete"], result)
        self.assertEqual(result["groups"][0]["verification_processes"], {"one": ["a", "b"]})
        self.assertIsNone(result["total_verification_invocations"])
        metadata = {"agent": "claude"}
        (self.root / "metadata.json").write_text(json.dumps(metadata))
        usage = collect_usage.collect(self.root, metadata, receipt, self.classifier)
        self.assertTrue(usage["valid_evidence"], usage)
        self.assertEqual(usage["verification_invocations"]["observed_verification_invocations"], 1)
        self.assertIsNone(usage["agent_verification_invocations"])

    def test_repeated_identical_commands_keep_group_identity_without_guessing_pairs(self):
        command = "go test ./..."
        events = []
        for name in ("first", "second"):
            events += self.invocation(name, command)
            events += self.command(name + "-go", ["/native/go", "test", "./..."], name)
        receipt = self.receipt([("tool2", command), ("tool1", command)], events)
        result = self.collect(receipt)
        self.assertTrue(result["association_complete"], result)
        self.assertEqual(result["observed_verification_invocations"], 2)
        self.assertEqual(len(result["groups"]), 1)
        self.assertEqual(result["groups"][0]["captured_invocations"], ["first", "second"])
        self.assertEqual([item["tool_call_id"] for item in result["groups"][0]["native_tool_calls"]], ["tool2", "tool1"])

    def test_denied_or_mismatched_request_does_not_validate_an_observed_process(self):
        command = "go test ./..."
        events = [*self.invocation("parent", command), *self.command("go", ["/native/go", "test", "./..."], "parent")]
        for commands, denied in (([("tool", command)], ["tool"]), ([("tool", "go vet ./...")], []),
                                 ([("a", command), ("b", command)], [])):
            with self.subTest(commands=commands, denied=denied):
                result = self.collect(self.receipt(commands, events, denied))
                self.assertEqual(result["observed_verification_invocations"], 0)
                self.assertFalse(result["association_complete"])
                self.assertTrue(result["unknown"])

    def test_failed_launch_is_not_execution_but_started_failure_and_interruption_are(self):
        command = "go test ./..."
        for state, code, expected, complete in (("not_started", 0, 0, True), ("complete", 1, 1, True),
                                                 ("incomplete", 0, 1, False)):
            with self.subTest(state=state):
                events = [*self.invocation("parent", command),
                          *self.command("go", ["/native/go", "test", "./..."], "parent", state=state, code=code)]
                result = self.collect(self.receipt([("tool", command)], events))
                self.assertEqual(result["observed_verification_invocations"], expected, result)
                self.assertEqual(result["association_complete"], complete, result)
                self.assertIsNone(result["total_verification_invocations"])

    def test_missing_or_dynamic_parent_and_non_verification_remain_distinct(self):
        command = "go test ./..."
        for parent, script, argv, expected_unknown in (
                (None, "eval 'go test ./...'", ["/native/go", "test", "./..."], True),
                ("parent", 'eval "$DYNAMIC"', ["/native/go", "test", "./..."], True),
                ("parent", "eval 'go test ./...'", ["/native/go", "version"], False)):
            with self.subTest(parent=parent, script=script, argv=argv):
                events = [*self.command("parent", ["/bin/zsh", "-c", script], role="shell_invocation"),
                          *self.command("native", argv, parent)]
                result = self.collect(self.receipt([("tool", command)], events))
                self.assertEqual(result["observed_verification_invocations"], 0, result)
                self.assertEqual(bool(result["unknown"]), expected_unknown, result)

    def test_corrupt_trace_ancestry_or_source_is_unknown(self):
        command = "go test ./..."
        for parent, origin, state in (("go", "agent", "complete"), (None, "taskbelay", "complete"),
                                     (None, "agent", "not_started")):
            with self.subTest(parent=parent, origin=origin, state=state):
                events = [*self.invocation("parent", command, parent=parent, origin=origin, state=state),
                          *self.command("go", ["/native/go", "test", "./..."], "parent")]
                result = self.collect(self.receipt([("tool", command)], events))
                self.assertIsNone(result["observed_verification_invocations"], result)
                self.assertFalse(result["association_complete"])
        receipt = self.receipt([("tool", command)], self.invocation("parent", command))
        changed = copy.deepcopy(receipt)
        changed["command_trace"]["records"]["observed_starts"] = 99
        self.assertIsNone(self.collect(changed)["observed_verification_invocations"])
        (self.root / "command-trace.jsonl").write_text("changed")
        self.assertIsNone(self.collect(receipt)["observed_verification_invocations"])

    def test_old_capture_unsupported_agent_and_missing_classifier_cannot_claim_zero(self):
        receipt = self.receipt([], self.command("legacy", ["/native/go", "test", "./..."]))
        for agent, classifier in (("claude", self.classifier), ("codex", self.classifier), ("claude", None)):
            with self.subTest(agent=agent, classifier=classifier):
                result = collect_invocations.collect(self.root, agent, receipt, classifier)
                self.assertIsNone(result["observed_verification_invocations"])
                self.assertFalse(result["association_complete"])


    def test_node_hosted_npx_descendants_are_grouped_by_native_tool_call(self):
        command = "npx vitest run"
        events = []
        for name in ("first", "second"):
            events += self.invocation(name, command)
            events += self.command(name + "-npx", ["/native/node", "/tools/npx", "vitest", "run"], name)
            events += self.command(name + "-nested-npx", ["/native/node", "/tools/npx", "vitest", "run"], name + "-npx")
            events += self.command(name + "-vitest", ["/native/node", "/modules/vitest/vitest.mjs", "run"], name + "-nested-npx")
        result = self.collect(self.receipt([("tool-1", command), ("tool-2", command)], events))
        self.assertTrue(result["association_complete"], result)
        self.assertEqual(result["observed_verification_invocations"], 2, result)
        self.assertEqual(result["groups"][0]["verification_processes"], {
            "first": ["first-npx", "first-nested-npx"], "second": ["second-npx", "second-nested-npx"]})
        self.assertIsNone(result["total_verification_invocations"])

    def test_node_hosted_npx_failed_start_and_denial_do_not_count(self):
        command = "npx vitest run"
        for state, denied, expected, complete in (("not_started", (), 0, True),
                                                 ("complete", (), 1, True),
                                                 ("complete", ("tool",), 0, False)):
            with self.subTest(state=state, denied=denied):
                events = [*self.invocation("parent", command),
                          *self.command("npx", ["/native/node", "/tools/npx", "vitest", "run"],
                                        "parent", state=state, code=1)]
                result = self.collect(self.receipt([("tool", command)], events, denied))
                self.assertEqual(result["observed_verification_invocations"], expected, result)
                self.assertEqual(result["association_complete"], complete, result)
                self.assertIsNone(result["total_verification_invocations"])



if __name__ == "__main__":
    unittest.main()
