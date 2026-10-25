import copy
import hashlib
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest

import collect_phase_costs as costs


class PhaseCostTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.workspace = self.root / "工作区 space"
        self.workspace.mkdir()
        self.other = self.root / "other"
        self.other.mkdir()
        self.database = self.root / "ledger.sqlite"

    def create(self):
        connection = sqlite3.connect(self.database)
        self.addCleanup(connection.close)
        connection.executescript("CREATE TABLE workspaces(id TEXT,path TEXT);"
                                "CREATE TABLE tasks(id TEXT,workspace_id TEXT);"
                                "CREATE TABLE checks(seq INTEGER PRIMARY KEY,task_id TEXT,depth TEXT,status TEXT,data TEXT);")
        connection.executemany("INSERT INTO workspaces VALUES(?,?)", [("w", str(self.workspace)), ("o", str(self.other))])
        connection.executemany("INSERT INTO tasks VALUES(?,?)", [("t", "w"), ("other", "o")])
        connection.commit()
        return connection

    def stage(self, started=True, unit="bytes"):
        return {"name": "baseline", "result": {"duration_ns": 100, "exit_code": 0,
                "process": {"started": started, "duration_ns": 80, **({"resources": {
                    "user_cpu_ns": 7, "system_cpu_ns": 2, "reported_maxrss": 20, "maxrss_unit": unit}}
                    if started else {})}}}

    def row(self, sequence, stages, task="t"):
        check = {"id": 0, "input": {"task_id": task}, "depth": "tests", "report": {"dynamic": {
            "execution_id": "same-tool-fingerprint", "stages": stages,
            "observations": [{"baseline": self.stage()["result"]}], "groups": [{"runs": [self.stage()["result"]]}]}}}
        return {"seq": sequence, "task_id": task, "depth": "tests", "status": "partial", "data": json.dumps(check)}

    def collect(self, rows):
        value = {"schema_version": 1, "workspace": str(self.workspace), "after_sequence": 1,
                 "through_sequence": 9, "rows": rows}
        data = json.dumps(value).encode()
        (self.root / "phase-checks.json").write_bytes(data)
        self.receipt = {"status": "recorded", "file": "phase-checks.json", "sha256": hashlib.sha256(data).hexdigest()}
        return costs.collect(self.root, self.receipt, str(self.workspace))

    def test_stages_count_actual_starts_and_keep_native_units_without_nested_duplicates(self):
        failed = self.stage()
        failed["result"]["exit_code"] = 7
        result = self.collect([self.row(2, [self.stage(), self.stage(False), self.stage(unit="KiB"), failed]),
                               self.row(3, [self.stage()])])
        phase = result["phases"]["baseline"]
        self.assertEqual(result["status"], "recorded")
        self.assertEqual(phase["recorded_command_attempts"], 5)
        self.assertEqual(phase["direct_starts"], 4)
        self.assertEqual(phase["sum_command_duration_ns"], 500)
        self.assertEqual(phase["native_resources"], {"user_cpu_ns": 28, "system_cpu_ns": 8,
                                                   "largest_reported_maxrss_bytes": 20480})
        self.assertFalse(result["complete_runtime_cost"])
        self.assertEqual([row["sequence"] for row in result["checks"]], [2, 3])

    def test_legacy_and_windows_missing_resources_are_unknown_not_zero(self):
        legacy, windows = self.stage(), self.stage()
        legacy["result"].pop("process")
        windows["result"]["process"].pop("resources")
        result = self.collect([self.row(2, [legacy, windows])])
        phase = result["phases"]["baseline"]
        self.assertIsNone(phase["direct_starts"])
        self.assertEqual(phase["observed_direct_starts"], 1)
        self.assertEqual(phase["unknown_start_attempts"], 1)
        self.assertIsNone(phase["native_resources"])
        self.assertIsNone(phase["sum_process_attempt_duration_ns"])
        missing = self.collect([self.row(2, None)])
        self.assertEqual(missing["checks_without_stages"], [2])

    def test_duplicate_ids_bad_units_and_malformed_counters_invalidate_only_costs(self):
        row = self.row(2, [self.stage()])
        for rows in ([row, row], [self.row(1, [])], [self.row(10, [])]):
            self.assertEqual(self.collect(rows)["status"], "unknown")
        for value in (True, -1, 0.5, "100", None):
            stage = self.stage()
            stage["result"]["duration_ns"] = value
            self.assertEqual(self.collect([self.row(2, [stage])])["status"], "unknown")
        for stage in (self.stage(unit="unknown"), self.stage()):
            if stage["result"]["process"]["resources"]["maxrss_unit"] == "bytes":
                stage["result"]["process"]["started"] = False
            self.assertEqual(self.collect([self.row(2, [stage])])["status"], "unknown")

    def test_source_changes_and_wrong_workspace_cannot_be_reused(self):
        self.collect([self.row(2, [self.stage()])])
        self.assertEqual(costs.collect(self.root, self.receipt, str(self.other))["status"], "unknown")
        (self.root / "phase-checks.json").write_text("changed")
        self.assertEqual(costs.collect(self.root, self.receipt, str(self.workspace))["status"], "unknown")
        self.assertEqual(costs.collect(self.root, None, str(self.workspace))["status"], "unknown")

    def test_readonly_capture_excludes_prior_cached_records_and_other_workspaces(self):
        connection = self.create()
        old = self.row(1, [self.stage()])
        connection.execute("INSERT INTO checks VALUES(:seq,:task_id,:depth,:status,:data)", old)
        connection.commit()
        before = costs.begin(self.database, self.workspace)
        for row in (self.row(2, [self.stage()]), self.row(3, [self.stage()], "other")):
            connection.execute("INSERT INTO checks VALUES(:seq,:task_id,:depth,:status,:data)", row)
        connection.commit()
        original = self.database.read_bytes()
        receipt = costs.capture(self.database, self.workspace, before, self.root)
        report = costs.collect(self.root, receipt, str(self.workspace))
        self.assertEqual([row["sequence"] for row in report["checks"]], [2])
        self.assertEqual(report["phases"]["baseline"]["direct_starts"], 1)
        self.assertEqual(self.database.read_bytes(), original)
        # Re-reading a cached report inserts no new row, so it adds no recorded stage cost.
        empty = self.root / "empty"
        empty.mkdir()
        cursor = costs.begin(self.database, self.workspace)
        receipt = costs.capture(self.database, self.workspace, cursor, empty)
        self.assertEqual(costs.collect(empty, receipt, str(self.workspace))["checks"], [])

    def test_missing_initial_ledger_is_allowed_but_unreadable_or_replaced_is_unknown(self):
        before = costs.begin(self.database, self.workspace)
        self.assertEqual(before["status"], "missing")
        self.assertEqual(costs.capture(self.database, self.workspace, before, self.root)["status"], "unknown")
        self.create()
        self.assertEqual(costs.capture(self.database, self.workspace, before, self.root)["status"], "recorded")
        before = costs.begin(self.database, self.workspace)
        changed = copy.deepcopy(before)
        changed["identity"][1] += 1
        self.assertEqual(costs.capture(self.database, self.workspace, changed, self.root)["status"], "unknown")
        self.assertEqual(costs.capture(self.database, self.workspace, {"status": "unknown"}, self.root)["status"], "unknown")

    def test_removed_foreign_workspace_does_not_hide_target_checks(self):
        connection = self.create()
        connection.execute("UPDATE workspaces SET path=? WHERE id='o'", (str(self.root / "removed"),))
        for row in (self.row(2, [self.stage()]), self.row(3, None, "other")):
            connection.execute("INSERT INTO checks VALUES(:seq,:task_id,:depth,:status,:data)", row)
        connection.commit()
        state = costs.snapshot(self.database, self.workspace, 0)
        self.assertEqual([row["seq"] for row in state["rows"]], [2])
        with self.assertRaises(OSError):
            costs.snapshot(self.database, self.root / "missing-target", 0)


if __name__ == "__main__":
    unittest.main()
