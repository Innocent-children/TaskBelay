"""Associate observed verification starts with native Claude Bash invocations."""

from collections import defaultdict
import hashlib
import json
from pathlib import Path
import subprocess

import dataset
import trace_command


def digest_text(value):
    return hashlib.sha256(value.encode()).hexdigest()


def bash_requests(root, receipt):
    requests, denied, evidence = {}, set(), {}
    for turn in receipt["turns"]:
        path = root / f"turn-{turn['turn']}.jsonl"
        evidence[path.name] = dataset.digest(path)
        for line in path.read_text().splitlines():
            if not line.strip():
                continue
            event = json.loads(line)
            if event.get("type") == "assistant":
                for part in event.get("message", {}).get("content", []):
                    if part.get("type") == "tool_use" and part.get("name") == "Bash":
                        identity = (turn["turn"], part["id"])
                        command = part["input"]["command"]
                        if identity in requests or not isinstance(command, str) or not command:
                            raise ValueError("duplicate or invalid native Bash request")
                        requests[identity] = {"turn": turn["turn"], "tool_call_id": part["id"],
                                              "command_sha256": digest_text(command)}
            if event.get("type") == "result":
                for denial in event.get("permission_denials", []):
                    if denial.get("tool_name") == "Bash":
                        denied.add((turn["turn"], denial["tool_use_id"]))
        if dataset.digest(path) != evidence[path.name]:
            raise ValueError("native Bash evidence changed while reading")
    grouped = defaultdict(list)
    for identity, request in requests.items():
        if identity not in denied:
            grouped[request["command_sha256"]].append(request)
    return grouped, evidence


def collect(root, agent, receipt, classifier):
    result = {"observed_verification_invocations": None, "association_complete": False,
              "groups": [], "unknown": [], "total_verification_invocations": None,
              "scope": "selected forwarded tools only; unmatched or unforwarded executions do not establish a total"}
    try:
        if agent != "claude":
            raise ValueError("native invocation capture is not verified for this Agent")
        if classifier is None:
            raise ValueError("recorded-command classifier was not supplied")
        root, classifier = Path(root), Path(classifier).resolve(strict=True)
        recorded = receipt["command_trace"]
        name = recorded["file"]
        if not isinstance(name, str) or Path(name).name != name:
            raise ValueError("invalid command trace path")
        trace = root / name
        if dataset.digest(trace) != recorded["sha256"]:
            raise ValueError("command trace bytes differ from receipt")
        parsed = trace_command.collect(trace)
        if parsed != recorded["records"] or dataset.digest(trace) != recorded["sha256"]:
            raise ValueError("command trace records changed")
        calls = {call["id"]: call for call in parsed["calls"]}
        for identity, call in calls.items():
            seen, parent = {identity}, call["attempt"]["parent_id"]
            while parent:
                if parent in seen:
                    raise ValueError("cyclic invocation ancestry")
                seen.add(parent)
                ancestor = calls[parent]
                if call["start"] is not None and ancestor["start"] is None:
                    raise ValueError("started command has an unstarted ancestor")
                parent = ancestor["attempt"]["parent_id"]
        if not any(call["attempt"].get("role") == "shell_invocation" for call in calls.values()):
            raise ValueError("trace has no captured shell invocation identities")
        inputs = [{"id": call["id"], "role": call["attempt"].get("role", "native"),
                   "argv": call["attempt"]["argv"], "directory": call["attempt"]["cwd"]}
                  for call in calls.values()]
        payload = json.dumps(inputs, ensure_ascii=False).encode()
        binary_hash = dataset.digest(classifier)
        execution = subprocess.run([str(classifier)], input=payload, capture_output=True, timeout=30)
        if execution.returncode or dataset.digest(classifier) != binary_hash:
            raise ValueError("recorded-command classifier failed or changed")
        classifications = json.loads(execution.stdout)
        by_id = {item["id"]: item for item in classifications}
        if len(by_id) != len(classifications) or set(by_id) != set(calls):
            raise ValueError("classifier results do not match trace identities")
        requests, native_evidence = bash_requests(root, receipt)
        invocations = defaultdict(list)
        for identity, call in calls.items():
            if (call["attempt"].get("role") == "shell_invocation"
                    and call["attempt"]["origin"] == "agent" and call["start"] is not None):
                literals = by_id[identity]["literal_commands_sha256"]
                if len(literals) == 1:
                    invocations[literals[0]].append(identity)
        verified = defaultdict(list)
        for identity, call in calls.items():
            if call["attempt"].get("role", "native") != "native" or call["attempt"]["origin"] != "agent" or call["start"] is None:
                continue
            classification = by_id[identity]
            if classification["verification"] is None:
                result["unknown"].append("unclassified observed command: " + identity)
                continue
            if classification["verification"] is not True:
                continue
            parent = call["attempt"]["parent_id"]
            while parent:
                ancestor = calls[parent]["attempt"]
                if ancestor["origin"] != "agent":
                    raise ValueError("Agent verification has a non-Agent ancestor")
                if ancestor.get("role") == "shell_invocation":
                    break
                parent = ancestor["parent_id"]
            if parent:
                verified[parent].append(identity)
            else:
                result["unknown"].append("verification start has no invocation parent: " + identity)
        matched = set()
        for command_hash, identities in invocations.items():
            candidates = requests.get(command_hash, [])
            observed = [identity for identity in identities if identity in verified]
            if not observed:
                continue
            if len(identities) != len(candidates):
                result["unknown"].append("native Bash and captured invocation counts differ: " + command_hash)
                continue
            # Identical commands may occur in several tool calls. Their equal
            # cardinality proves distinct invocations, not a fabricated ID pairing.
            result["groups"].append({"command_sha256": command_hash, "native_tool_calls": candidates,
                                     "captured_invocations": identities, "verification_invocations": observed,
                                     "verification_processes": {identity: verified[identity] for identity in observed}})
            matched.update(observed)
        for identity in verified.keys() - matched:
            result["unknown"].append("verification invocation lacks a native Bash match: " + identity)
        if not parsed["complete"]:
            result["unknown"].append("command trace is incomplete")
        result.update(observed_verification_invocations=len(matched), association_complete=not result["unknown"],
                      evidence={"trace_sha256": recorded["sha256"], "native_outputs_sha256": native_evidence,
                                "classifier_sha256": binary_hash, "input_sha256": hashlib.sha256(payload).hexdigest(),
                                "output_sha256": hashlib.sha256(execution.stdout).hexdigest(),
                                "collector_sha256": dataset.digest(Path(__file__))})
    except (OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.TimeoutExpired) as error:
        result["unknown"].append(str(error))
    return result
