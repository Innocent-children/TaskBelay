"""Collect only newly persisted workspace stages; never infer complete runtime cost."""

from contextlib import closing
import hashlib
import json
import os
from pathlib import Path
import sqlite3


PHASES = ("prepare", "compile", "baseline", "mutation")


def snapshot(database, workspace, after=None):
    path = Path(database).resolve()
    if not path.exists():
        return {"status": "missing", "cursor": 0, "identity": None, "rows": []}
    identity = path.stat()
    Path(workspace).stat()
    with closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True, timeout=0.2)) as connection:
        connection.row_factory = sqlite3.Row
        connection.execute("BEGIN")
        cursor = connection.execute("SELECT COALESCE(MAX(seq),0) FROM checks").fetchone()[0]
        identities = []
        if after is not None:
            for row in connection.execute("SELECT id,path FROM workspaces"):
                try:
                    if os.path.samefile(row["path"], workspace):
                        identities.append(row["id"])
                except FileNotFoundError:
                    continue
        selected = []
        if identities:
            placeholders = ",".join("?" for _ in identities)
            rows = connection.execute(
                "SELECT c.seq,c.task_id,c.depth,c.status,c.data FROM checks c "
                "JOIN tasks t ON t.id=c.task_id WHERE c.seq>? AND c.seq<=? "
                "AND c.depth='tests' AND t.workspace_id IN (" + placeholders + ") ORDER BY c.seq",
                (after, cursor, *identities)).fetchall()
            selected = [dict(row) for row in rows]
    final_identity = path.stat()
    if (identity.st_dev, identity.st_ino) != (final_identity.st_dev, final_identity.st_ino):
        raise ValueError("check ledger changed while taking the snapshot")
    return {"status": "recorded", "cursor": cursor, "identity": [identity.st_dev, identity.st_ino],
            "rows": selected}


def begin(database, workspace):
    try:
        return snapshot(database, workspace)
    except (OSError, ValueError, sqlite3.Error) as error:
        return {"status": "unknown", "error": str(error)}


def capture(database, workspace, before, destination):
    try:
        if before["status"] not in {"missing", "recorded"}:
            raise ValueError("initial check cursor is unavailable")
        after = snapshot(database, workspace, before["cursor"])
        if after["status"] != "recorded":
            raise ValueError("no check ledger was available at collection")
        if after["cursor"] < before["cursor"] or (before["identity"] is not None and
                before["identity"] != after["identity"]):
            raise ValueError("check ledger was replaced or its sequence moved backwards")
        value = {"schema_version": 1, "workspace": str(Path(workspace).resolve()),
                 "after_sequence": before["cursor"], "through_sequence": after["cursor"], "rows": after["rows"]}
        data = (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")
        path = Path(destination) / "phase-checks.json"
        with path.open("xb") as stream:
            os.chmod(path, 0o600)
            stream.write(data)
        return {"status": "recorded", "file": path.name, "sha256": hashlib.sha256(data).hexdigest()}
    except (OSError, ValueError, KeyError, TypeError, sqlite3.Error) as error:
        return {"status": "unknown", "error": str(error)}


def integer(value):
    if type(value) is not int or value < 0:
        raise ValueError("expected a nonnegative integer in phase evidence")
    return value


def summarize(stages):
    started, unknown, user, system, peaks = 0, 0, [], [], []
    durations, process_durations = [], []
    resources_complete = True
    for result in stages:
        durations.append(integer(result["duration_ns"]))
        process = result.get("process")
        if process is None:
            unknown += 1
            resources_complete = False
            continue
        if type(process["started"]) is not bool:
            raise ValueError("invalid process start evidence")
        process_durations.append(integer(process["duration_ns"]))
        usage = process.get("resources")
        if not process["started"]:
            if usage is not None:
                raise ValueError("unstarted process has resource evidence")
            continue
        started += 1
        if usage is None:
            resources_complete = False
            continue
        user.append(integer(usage["user_cpu_ns"]))
        system.append(integer(usage["system_cpu_ns"]))
        unit = usage["maxrss_unit"]
        if unit not in {"bytes", "KiB"}:
            raise ValueError("unrecognized native maximum RSS unit")
        peaks.append(integer(usage["reported_maxrss"]) * (1024 if unit == "KiB" else 1))
    return {"recorded_command_attempts": len(stages), "observed_direct_starts": started,
            "unknown_start_attempts": unknown, "direct_starts": None if unknown else started,
            "sum_command_duration_ns": sum(durations),
            "sum_process_attempt_duration_ns": None if unknown else sum(process_durations),
            "native_resources": {"user_cpu_ns": sum(user), "system_cpu_ns": sum(system),
                                 "largest_reported_maxrss_bytes": max(peaks, default=None)}
            if resources_complete else None}


def collect(directory, recorded, workspace):
    result = {"status": "unknown", "scope": "new_persisted_workspace_check_stages",
              "phases": {phase: None for phase in PHASES}, "complete_runtime_cost": False,
              "errors": [], "limitations": [
                  "workspace records do not identify the calling Agent session",
                  "cached reports are not new executions; identity probes and Git/export work are not fully recorded",
                  "command durations may overlap and are not total wall time",
                  "direct starts do not count descendants; native CPU may include waited descendants",
                  "largest reported RSS is not a concurrent process-tree memory peak",
                  "unpublished checks and commands without recorded stages remain outside this scope",
              ]}
    try:
        if not isinstance(recorded, dict) or recorded.get("status") != "recorded":
            raise ValueError("no persisted phase evidence was collected")
        if recorded["file"] != "phase-checks.json":
            raise ValueError("unexpected phase evidence file")
        path = Path(directory) / recorded["file"]
        data = path.read_bytes()
        if hashlib.sha256(data).hexdigest() != recorded["sha256"]:
            raise ValueError("phase evidence digest changed")
        value = json.loads(data)
        if value["schema_version"] != 1 or value["workspace"] != workspace:
            raise ValueError("phase evidence belongs to a different workspace or schema")
        first, last = integer(value["after_sequence"]), integer(value["through_sequence"])
        if first > last or not isinstance(value["rows"], list):
            raise ValueError("invalid phase evidence sequence range")
        phases, seen, missing, records = {phase: [] for phase in PHASES}, set(), [], []
        for row in value["rows"]:
            sequence = integer(row["seq"])
            if sequence <= first or sequence > last or sequence in seen or row["depth"] != "tests":
                raise ValueError("duplicate or out-of-range dynamic check")
            seen.add(sequence)
            check = json.loads(row["data"])
            if check["input"]["task_id"] != row["task_id"] or check["depth"] != "tests":
                raise ValueError("check identity differs from ledger row")
            dynamic = check["report"].get("dynamic")
            records.append({"sequence": sequence, "task_id": row["task_id"], "status": row["status"]})
            if dynamic is None or dynamic.get("stages") is None:
                missing.append(sequence)
                continue
            if not isinstance(dynamic["stages"], list):
                raise ValueError("invalid dynamic stage list")
            for stage in dynamic["stages"]:
                if not isinstance(stage["name"], str) or not stage["name"]:
                    raise ValueError("invalid stage name")
                phases.setdefault(stage["name"], []).append(stage["result"])
        summaries = {name: summarize(stages) for name, stages in phases.items()}
        if path.read_bytes() != data:
            raise ValueError("phase evidence changed during collection")
        result.update(status="recorded", phases=summaries, checks=records,
                      checks_without_stages=missing, source_sha256=recorded["sha256"])
    except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
        result["errors"].append(str(error))
    return result
