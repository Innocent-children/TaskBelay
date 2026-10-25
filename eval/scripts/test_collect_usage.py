import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import collect_usage
import dataset
from run_agents import parse_events
import trace_command


class UsageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def write(self, name, data):
        (self.root / name).write_text(json.dumps(data))

    def agent(self, agent, reports):
        metadata = {"agent": agent}
        receipt = {"turns": []}
        for number, usage in enumerate(reports, 1):
            event = ({"type": "result", "subtype": "success", "usage": usage} if agent == "claude" else
                     {"type": "turn.completed", "usage": usage})
            path = self.root / f"turn-{number}.jsonl"
            path.write_text(json.dumps(event) + "\n")
            receipt["turns"].append({"turn": number, "duration_seconds": number,
                                     "events": parse_events(path, agent)})
        self.write("metadata.json", metadata)
        self.write("result.json", receipt)
        return metadata, receipt

    def trace(self, events):
        path = self.root / "command-trace.jsonl"
        path.write_text("".join(json.dumps(event) + "\n" for event in events))
        return {"status": "recorded", "file": path.name, "sha256": dataset.digest(path),
                "records": trace_command.collect(path)}

    def attempt(self, identity, parent=None, origin="agent"):
        return {"event": "attempt", "id": identity, "parent_id": parent, "origin": origin,
                "argv": ["/native/go", "test", "./..."], "cwd": "/workspace"}

    def end(self, identity, seconds, peak=1024, unit="bytes"):
        return {"event": "exit", "id": identity, "returncode": 0, "elapsed_seconds": seconds,
                "resources": {"user_seconds": seconds / 2, "system_seconds": seconds / 4,
                              "reported_maxrss": peak, "maxrss_unit": unit}}

    def test_terminal_counters_ignore_nested_iteration_and_cache_subtotals(self):
        usage = {"input_tokens": 2, "cache_creation_input_tokens": 3, "cache_read_input_tokens": 5,
                 "output_tokens": 7, "iterations": [{"input_tokens": 1000}],
                 "cache_creation": {"ephemeral_1h_input_tokens": 3}, "output_tokens_details": {"thinking_tokens": 4}}
        metadata, receipt = self.agent("claude", [usage, usage])
        result = collect_usage.collect(self.root, metadata, receipt)
        self.assertTrue(result["valid_evidence"], result["errors"])
        self.assertEqual(result["model_tokens"]["native_field_totals"], {
            "input_tokens": 4, "cache_creation_input_tokens": 6, "cache_read_input_tokens": 10, "output_tokens": 14})
        self.assertEqual(result["invocation_wall_seconds"], 3)
        self.assertIsNone(result["agent_verification_invocations"])
        self.assertIsNone(result["taskbelay_feedback"])
        self.assertIsNone(result["user_confirmations"])
        self.assertFalse(result["full_cost_complete"])
        self.assertEqual(result["command_trace"]["status"], "unknown")

    def test_codex_cache_counter_is_separate_and_missing_fields_are_not_zero(self):
        metadata, receipt = self.agent("codex", [{"input_tokens": 20, "cached_input_tokens": 10, "output_tokens": 3},
                                                  {"input_tokens": 4, "output_tokens": 2}])
        result = collect_usage.collect(self.root, metadata, receipt)
        self.assertEqual(result["model_tokens"]["native_field_totals"], {
            "input_tokens": 24, "cached_input_tokens": None, "output_tokens": 5})
        self.assertEqual(result["model_tokens"]["missing_or_invalid"], [{"turn": 2, "field": "cached_input_tokens"}])

    def test_duplicate_terminal_usage_and_invalid_counters_remain_unknown(self):
        for value in (True, -1, 1.5, None, "2"):
            with self.subTest(value=value):
                metadata, receipt = self.agent("codex", [{"input_tokens": value, "cached_input_tokens": 0, "output_tokens": 2}])
                self.assertIsNone(collect_usage.collect(self.root, metadata, receipt)["model_tokens"]["native_field_totals"]["input_tokens"])
        path = self.root / "turn-1.jsonl"
        path.write_text(path.read_text() * 2)
        receipt["turns"][0]["events"] = parse_events(path, "codex")
        self.write("result.json", receipt)
        result = collect_usage.collect(self.root, metadata, receipt)
        self.assertTrue(all(value is None for value in result["model_tokens"]["native_field_totals"].values()))

    def test_changed_native_output_or_receipt_is_rejected(self):
        metadata, receipt = self.agent("claude", [{"input_tokens": 3}])
        changed = copy.deepcopy(receipt)
        changed["turns"][0]["events"]["usage"][0]["input_tokens"] = 99
        self.write("result.json", changed)
        result = collect_usage.collect(self.root, metadata, changed)
        self.assertFalse(result["valid_evidence"])
        self.assertIn("differs from native output", result["errors"][0])
        self.assertFalse(collect_usage.collect(self.root, metadata, receipt)["valid_evidence"])

    def test_native_output_change_during_invocation_collection_invalidates_cost_evidence(self):
        metadata, receipt = self.agent("claude", [{"input_tokens": 3}])

        def change_output(*_args):
            (self.root / "turn-1.jsonl").write_text("changed during classification")
            return {"observed_verification_invocations": 1}

        with mock.patch.object(collect_usage.collect_invocations, "collect", side_effect=change_output):
            result = collect_usage.collect(self.root, metadata, receipt)
        self.assertFalse(result["valid_evidence"], result)
        self.assertIsNone(result["model_tokens"])
        self.assertTrue(result["errors"])

    def test_nested_costs_launch_failures_and_origin_attribution_are_distinct(self):
        events = [self.attempt("parent", origin="taskbelay"), {"event": "start", "id": "parent"},
                  self.attempt("child", "parent", "assessment"), {"event": "start", "id": "child"},
                  self.end("child", 3, 2, "KiB"), self.end("parent", 10, 4096),
                  self.attempt("failed"), {"event": "launch_failed", "id": "failed", "error": "missing tool"}]
        result = collect_usage.traced_usage(self.root, self.trace(events))
        self.assertEqual(result["observed_native_starts"], 2)
        product = result["origins"]["taskbelay"]
        self.assertEqual(product["root_inclusive_resources"]["inclusive_elapsed_seconds_sum"], 10)
        self.assertEqual(product["root_inclusive_resources"]["reported_user_seconds_sum"], 5)
        self.assertEqual(result["origins"]["assessment"]["nested_call_ids"], ["child"])
        self.assertEqual(result["origins"]["agent"]["not_started"], 1)
        self.assertEqual(result["origins"]["agent"]["observed_native_starts"], 0)

    def test_missing_termination_or_resources_never_manufacture_complete_cost(self):
        events = [self.attempt("a"), {"event": "start", "id": "a"}]
        result = collect_usage.traced_usage(self.root, self.trace(events))
        self.assertEqual(result["status"], "partial")
        self.assertEqual(result["observed_native_starts"], 1)
        self.assertTrue(all(value is None for value in result["origins"]["agent"]["root_inclusive_resources"].values()))
        end = self.end("a", 2)
        end["resources"] = None
        result = collect_usage.traced_usage(self.root, self.trace([*events, end]))
        self.assertEqual(result["status"], "recorded")
        self.assertIsNone(result["origins"]["agent"]["root_inclusive_resources"]["reported_user_seconds_sum"])
        self.assertEqual(result["origins"]["agent"]["root_inclusive_resources"]["inclusive_elapsed_seconds_sum"], 2)

    def test_trace_hash_summary_and_ancestry_must_match(self):
        recorded = self.trace([self.attempt("a"), {"event": "start", "id": "a"}, self.end("a", 1)])
        changed = copy.deepcopy(recorded)
        changed["records"]["observed_starts"] = 9
        self.assertEqual(collect_usage.traced_usage(self.root, changed)["status"], "unknown")
        (self.root / recorded["file"]).write_text("changed")
        self.assertIsNone(collect_usage.traced_usage(self.root, recorded)["observed_native_starts"])
        cyclic = self.trace([self.attempt("a", "b"), self.attempt("b", "a")])
        self.assertIn("cyclic", collect_usage.traced_usage(self.root, cyclic)["errors"][0])
        unstarted = self.trace([self.attempt("a"), self.attempt("b", "a"), {"event": "start", "id": "b"}])
        self.assertIn("ancestor", collect_usage.traced_usage(self.root, unstarted)["errors"][0])

    def test_malformed_trace_receipt_is_unknown_without_crashing_collection(self):
        for recorded in ("trace", ["malformed"], 42):
            with self.subTest(recorded=recorded):
                result = collect_usage.traced_usage(self.root, recorded)
                self.assertEqual(result["status"], "unknown")
                self.assertIsNone(result["observed_native_starts"])

    def test_native_memory_units_and_bad_resource_numbers(self):
        end = self.end("a", 2, 4, "KiB")
        call = {"exit": end}
        self.assertEqual(collect_usage.resources([call])["largest_reported_maxrss_bytes"], 4096)
        end["resources"]["user_seconds"] = -1
        end["resources"]["maxrss_unit"] = "unknown"
        end["elapsed_seconds"] = float("nan")
        result = collect_usage.resources([call])
        self.assertIsNone(result["reported_user_seconds_sum"])
        self.assertIsNone(result["largest_reported_maxrss_bytes"])
        self.assertIsNone(result["inclusive_elapsed_seconds_sum"])
        self.assertIsNone(collect_usage.total([1e308, 1e308]))

    def test_interrupted_native_exit_still_has_cost_but_is_not_a_success(self):
        end = self.end("mcp", 2)
        end.update(returncode=-2, termination_signal=2)
        recorded = self.trace([self.attempt("mcp", origin="taskbelay"), {"event": "start", "id": "mcp"}, end])
        result = collect_usage.traced_usage(self.root, recorded)
        self.assertEqual(result["status"], "recorded")
        self.assertEqual(result["origins"]["taskbelay"]["observed_exit_codes"], {"-2": 1})
        self.assertEqual(result["origins"]["taskbelay"]["root_inclusive_resources"]["inclusive_elapsed_seconds_sum"], 2)


if __name__ == "__main__":
    unittest.main()
