import json
from pathlib import Path
import tempfile
import unittest

import collect_performance


class PerformanceEvidenceTests(unittest.TestCase):
    def fixture(self, root, truncated=False, incorrect=False):
        rows = [{"milliseconds": 50}, {"milliseconds": 150, "error": "process failed"},
                {"milliseconds": 200, "check_status": "partial", "missing_count": 12}]
        profile = {"name": "sample", "samples": rows, "budget_ms": 100,
                   "median_ms": 150, "p95_ms": 200, "over_budget": 2,
                   "errors": 1, "incomplete": 0 if incorrect else 1}
        lines = ["TASKBELAY_PERFORMANCE_SAMPLE " + json.dumps({
            "profile": "sample", "sample": i + 1, "measurement": row}) + "\n"
                 for i, row in enumerate(rows)]
        lines.append("TASKBELAY_PERFORMANCE_PROFILE " + json.dumps(profile) + "\n")
        text = "".join(lines)
        if truncated:
            text = text[:text.index("TASKBELAY_PERFORMANCE_PROFILE ")]
        path = root / "go.jsonl"
        with path.open("w") as stream:
            for start in range(0, len(text), 37):
                stream.write(json.dumps({"Action": "output", "Test": "TestPerformanceProfiles/sample",
                                         "Output": text[start:start + 37]}) + "\n")
        return path

    def test_fragmented_output_retains_errors_partial_and_percentiles(self):
        with tempfile.TemporaryDirectory() as root:
            report = collect_performance.collect(self.fixture(Path(root)))
        profile = report["profiles"][0]
        self.assertEqual((profile["errors"], profile["incomplete"], profile["over_budget"]), (1, 1, 2))
        self.assertEqual(profile["check_states"], {"partial": 1})
        self.assertEqual(profile["sample_count"], 3)
        self.assertEqual((profile["median_ms"], profile["p95_ms"]), (150, 200))
        self.assertEqual(profile["diagnostic_collection"], "not_recorded")

    def test_normal_exit_with_diagnostic_remains_degraded(self):
        with tempfile.TemporaryDirectory() as root:
            path = self.fixture(Path(root))
            text = "".join(json.loads(line)["Output"] for line in path.read_text().splitlines())
            text = text.replace('"error": "process failed"', '"diagnostics": ["database busy"]')
            text = text.replace('"errors": 1', '"errors": 0, "degraded": 1')
            path.write_text(json.dumps({"Action": "output", "Test": "profile", "Output": text}) + "\n")
            profile = collect_performance.collect(path)["profiles"][0]
        self.assertEqual((profile["errors"], profile["degraded"]), (0, 1))
        self.assertEqual(profile["diagnostic_collection"], "recorded")

    def test_incomplete_or_incorrect_summary_is_not_reported_as_valid(self):
        for options in [{"truncated": True}, {"incorrect": True}]:
            with self.subTest(options=options), tempfile.TemporaryDirectory() as root:
                with self.assertRaises(ValueError):
                    collect_performance.collect(self.fixture(Path(root), **options))


if __name__ == "__main__":
    unittest.main()
