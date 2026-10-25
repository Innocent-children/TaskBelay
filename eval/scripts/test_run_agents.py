import json
from contextlib import closing
import os
from pathlib import Path
import sqlite3
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

import run_agents


class AgentEvaluationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.workspace = self.root / "工作区 space"
        self.workspace.mkdir()
        (self.workspace / ".git").mkdir()

    def events(self, agent, events):
        path = self.root / "events.jsonl"
        path.write_text("\n".join(json.dumps(item) for item in events), encoding="utf-8")
        return run_agents.parse_events(path, agent)

    def test_codex_json_records_usage_tools_and_exact_resume_id(self):
        parsed = self.events("codex", [
            {"type": "thread.started", "thread_id": "exact-session"},
            {"type": "item.completed", "item": {"type": "command_execution"}},
            {"type": "turn.completed", "usage": {"input_tokens": 3, "output_tokens": 2}},
        ])
        self.assertEqual(parsed["session_id"], "exact-session")
        self.assertEqual(parsed["tool_events"], 1)
        self.assertEqual(parsed["usage"][0]["input_tokens"], 3)
        config = {"agent": "codex", "model": "fixed-model", "arguments": []}
        argv = run_agents.command(config, parsed["session_id"])
        self.assertEqual(argv[-2:], ["exact-session", "-"])
        self.assertNotIn("--last", argv)

    def test_claude_result_and_model_are_not_guessed_from_text(self):
        parsed = self.events("claude", [
            {"type": "system", "subtype": "init", "session_id": "s", "model": "fixed-model"},
            {"type": "assistant", "session_id": "s", "message": {"content": [
                {"type": "tool_use", "name": "Write"}, {"type": "text", "text": "all passed"}]}},
            {"type": "result", "subtype": "error_during_execution", "is_error": True, "session_id": "s"},
        ])
        self.assertFalse(parsed["completed"])
        self.assertEqual(parsed["models"], ["fixed-model"])
        self.assertEqual(parsed["tool_events"], 1)
        self.assertTrue(parsed["errors"])

    def test_missing_hooks_and_failed_hooks_never_count_as_valid_samples(self):
        events = {"session_id": "s", "completed": True, "errors": [], "tool_events": 1}
        hooks = [{"kind": kind, "state": "recorded", "error": ""}
                 for kind in ["session_start", "user_prompt", "stop", "edit_use"]]
        self.assertEqual(run_agents.integration(True, hooks, events, "s", 0, False), [])
        self.assertIn("missing_hook:stop", run_agents.integration(True, hooks[:2], events, "s", 0, False))
        hooks[-1]["state"] = "failed"
        self.assertIn("hook_not_recorded_successfully", run_agents.integration(True, hooks, events, "s", 0, False))
        self.assertIn("resume_changed_session", run_agents.integration(True, hooks, events, "other", 0, False))
        self.assertIn("disabled_run_has_taskbelay_hooks", run_agents.integration(False, hooks, events, None, 0, False))

    def test_read_only_hook_query_filters_workspace_agent_session_and_cursor(self):
        database = self.root / "taskbelay.db"
        with closing(sqlite3.connect(database)) as connection, connection:
            connection.executescript("CREATE TABLE workspaces(id TEXT,path TEXT); CREATE TABLE hook_events("
                                     "id INTEGER,workspace_id TEXT,agent TEXT,session_id TEXT,round_id TEXT,"
                                     "kind TEXT,tool_call_id TEXT,state TEXT,error TEXT,decision TEXT DEFAULT '{}');")
            connection.executemany("INSERT INTO workspaces VALUES(?,?)", [
                ("w", str(self.workspace)), ("elsewhere", str(self.root / "another"))])
            for row in [(1,"w","codex","s"),(2,"w","codex","other"),
                        (3,"elsewhere","codex","s"),(4,"w","claude","s"),(5,"w","codex","s")]:
                connection.execute("INSERT INTO hook_events(id,workspace_id,agent,session_id,round_id,kind,tool_call_id,state,error) VALUES(?,?,?,?,?,?,?,?,?)",
                                   (*row,"r","stop","","recorded",""))
        before = database.read_bytes()
        rows = run_agents.read_hooks(database, self.workspace, "codex", 1, "s")
        self.assertEqual([row["id"] for row in rows], [5])
        self.assertEqual(database.read_bytes(), before)

    def test_invalid_json_and_changed_session_are_invalid(self):
        parsed = self.events("codex", [{"type":"thread.started","thread_id":"a"},
                                      {"type":"thread.started","thread_id":"b"},
                                      {"type":"item.completed","item":"bad"}])
        self.assertEqual(len(parsed["errors"]), 2)

    def test_manifest_requires_fixed_model_and_preserves_runner_boundaries(self):
        config = {"agent":"codex","model":"fixed","enabled":True,"workspace":str(self.workspace),
                  "database":str(self.root / "taskbelay.db"),"turns":[{"prompt":"implement behavior"}]}
        self.assertEqual(run_agents.validate(config), self.workspace)
        for arguments in [["--last"], ["--model=changed"], ["--ephemeral"],
                          ["-c", "model=changed"], ["--config=sandbox_mode=other"],
                          ["--dangerously-bypass-approvals-and-sandbox"]]:
            with self.subTest(arguments=arguments), self.assertRaises(ValueError):
                run_agents.validate({**config,"arguments":arguments})
        with self.assertRaises(ValueError):
            run_agents.validate({**config,"model":""})

    def test_real_command_trace_is_preserved_separately_from_agent_integration(self):
        trace = self.root / "native.jsonl"
        config = {"agent": "claude", "model": "fixed", "enabled": False, "workspace": str(self.workspace),
                  "database": str(self.root / "absent.db"), "turns": [{"prompt": "fixture"}], "command_trace": str(trace)}
        forwarding = self.root / "forward.json"
        forwarding.write_text(json.dumps({"command": sys.executable, "trace": str(trace)}))
        native = [sys.executable, str(Path(run_agents.trace_command.__file__)), str(forwarding), "--", "-c", "import sys;sys.exit(7)"]
        events = [{"type": "system", "subtype": "init", "session_id": "fixture", "model": "fixed"},
                  {"type": "assistant", "session_id": "fixture", "message": {"content": [{"type": "tool_use"}]}},
                  {"type": "result", "subtype": "success", "session_id": "fixture", "usage": {}}]
        script = ("import json,subprocess\nsubprocess.run(" + repr(native) + ")\n" +
                  "for item in " + repr(events) + ": print(json.dumps(item))\n")
        destination = self.root / "evidence"
        with patch.object(run_agents, "agent_version", return_value="fixture-version"), \
                patch.object(run_agents, "command", return_value=[sys.executable, "-c", script]):
            report = run_agents.run(config, destination)
        self.assertTrue(report["valid_integration"])
        self.assertEqual(report["command_trace"]["status"], "recorded")
        self.assertEqual(report["command_trace"]["observed_starts"], 1)
        self.assertEqual(report["command_trace"]["records"]["calls"][0]["exit"]["returncode"], 7)
        self.assertEqual(trace.read_bytes(), (destination / "command-trace.jsonl").read_bytes())
        metadata = json.loads((destination / "metadata.json").read_text())
        self.assertEqual(metadata["trace_environment"]["PATH"], os.environ.get("PATH"))
        self.assertFalse(report["performance_comparison_evaluated"])
        with self.assertRaisesRegex(ValueError, "new, outside"):
            run_agents.validate(config)

    def test_missing_empty_and_unfinished_trace_never_mean_zero_executions(self):
        trace = self.root / "native.jsonl"
        for number, data in enumerate((None, "", '{"event":"attempt","id":"pending","parent_id":null}\n', "broken\n")):
            destination = self.root / str(number)
            destination.mkdir()
            if data is not None:
                trace.write_text(data)
            result = run_agents.capture_trace({"command_trace": str(trace)}, destination)
            self.assertEqual(result["status"], "unknown")
            self.assertIsNone(result["observed_starts"])
            self.assertIn("error", result)
        config = {"agent": "codex", "model": "fixed", "enabled": False, "workspace": str(self.workspace),
                  "database": str(self.root / "absent.db"), "turns": [{"prompt": "fixture"}],
                  "command_trace": str(self.workspace / "would-change-candidate.jsonl")}
        with self.assertRaisesRegex(ValueError, "outside"):
            run_agents.validate(config)

    @unittest.skipIf(os.name == "nt", "native Windows process-tree termination remains pending")
    def test_timeout_terminates_the_owned_process_group(self):
        marker = self.root / "child-survived"
        child = "import time; from pathlib import Path; time.sleep(1); Path(" + repr(str(marker)) + ").touch()"
        parent = "import subprocess,sys,time; subprocess.Popen([sys.executable,'-c'," + repr(child) + "]); time.sleep(30)"
        code, timed_out, elapsed = run_agents.execute([sys.executable, "-c", parent], "", self.workspace,
            self.root / "out.jsonl", self.root / "err.log", 0.2)
        self.assertTrue(timed_out)
        self.assertNotEqual(code, 0)
        self.assertLess(elapsed, 2)
        time.sleep(1.1)
        self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
