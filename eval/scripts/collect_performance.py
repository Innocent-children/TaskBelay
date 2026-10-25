"""Collect measured hook latency, including errors and incomplete checks."""

import argparse
from collections import Counter, defaultdict
import hashlib
import json
import math
from pathlib import Path
import statistics


def collect(path):
    outputs = defaultdict(list)
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        event = json.loads(line)
        if event.get("Action") == "output":
            outputs[event.get("Test", "")].append((number, event.get("Output", "")))
    profiles, samples = {}, defaultdict(list)
    for test, chunks in outputs.items():
        # go test -json can split one t.Log record across multiple Output events.
        for line in "".join(text for _, text in chunks).splitlines():
            if "TASKBELAY_PERFORMANCE_SAMPLE " in line:
                sample = json.loads(line.split("TASKBELAY_PERFORMANCE_SAMPLE ", 1)[1])
                samples[sample["profile"]].append(sample)
            if "TASKBELAY_PERFORMANCE_PROFILE " in line:
                profile = json.loads(line.split("TASKBELAY_PERFORMANCE_PROFILE ", 1)[1])
                name = profile["name"]
                if name in profiles:
                    raise ValueError("duplicate profile: " + name)
                profile["test"] = test
                profiles[name] = profile
    if not profiles or set(profiles) != set(samples):
        raise ValueError("missing complete performance profile")
    for name, profile in profiles.items():
        rows = profile["samples"]
        recorded = samples[name]
        if not rows or [row["sample"] for row in recorded] != list(range(1, len(rows) + 1)):
            raise ValueError("missing or repeated sample: " + name)
        if rows != [row["measurement"] for row in recorded]:
            raise ValueError("aggregate differs from observed samples: " + name)
        times = sorted(row["milliseconds"] for row in rows)
        if any(not math.isfinite(value) or value < 0 for value in times):
            raise ValueError("invalid elapsed time: " + name)
        computed = {
            "median_ms": statistics.median(times),
            "p95_ms": times[math.ceil(len(times) * .95) - 1],
            "over_budget": sum(value > profile["budget_ms"] for value in times),
            "errors": sum(bool(row.get("error")) for row in rows),
            "incomplete": sum(row.get("check_status", "complete") != "complete" for row in rows),
        }
        if "degraded" in profile:
            computed["degraded"] = sum(bool(row.get("diagnostics")) for row in rows)
        profile["diagnostic_collection"] = "recorded" if "degraded" in profile else "not_recorded"
        if any(not math.isclose(profile[key], value, rel_tol=1e-12, abs_tol=1e-9)
               for key, value in computed.items()):
            raise ValueError("incorrect aggregate: " + name)
        profile["sample_count"] = len(rows)
        profile["check_states"] = dict(Counter(row["check_status"] for row in rows if "check_status" in row))
    return {
        "source": str(path), "source_sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
        "profiles": list(profiles.values()),
        "percentile_method": "nearest rank: ceil(n * 0.95), median averages the middle pair",
        "limitations": [
            "Independent hook test processes include process startup; these are not native Agent timings.",
            "A test pass does not make errors or incomplete product checks pass.",
            "Hook errors can exit normally; recorded diagnostics and process errors are counted separately.",
            "One-sample probes do not establish a performance percentile.",
        ],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("log", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = collect(args.log)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("x", encoding="utf-8") as stream:
        json.dump(result, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    for profile in result["profiles"]:
        print(json.dumps({key: value for key, value in profile.items() if key != "samples"}))


if __name__ == "__main__":
    main()
