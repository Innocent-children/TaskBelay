"""Measure successfully written hook feedback without claiming client consumption."""

from collections import Counter
import hashlib
import json
from pathlib import Path
import subprocess

import dataset


DECISION_FIELDS = {"AdditionalContext", "DenyTool", "BlockStop", "UserMessage"}


def collect(metadata, receipt, renderer):
    result = {"status": "unknown", "scope": "recorded_hook_text_written_to_stdout",
              "observed": None, "total_taskbelay_feedback": None, "unknown": [],
              "limitations": [
                  "hook records do not prove client consumption or model attention",
                  "MCP responses and unrecorded failure messages are outside this scope",
                  "estimated text tokens are not native model usage; JSON envelope bytes are excluded",
                  "permission denials and hook blocks do not count as user confirmations",
              ]}
    try:
        if renderer is None:
            raise ValueError("product feedback renderer was not supplied")
        renderer = Path(renderer).resolve(strict=True)
        if metadata["agent"] not in {"claude", "codex"} or not isinstance(metadata.get("workspace"), str):
            raise ValueError("feedback Agent or workspace identity is missing")
        inputs, seen, unknown = [], set(), []
        for turn in receipt["turns"]:
            if not turn.get("session_id") or turn["events"]["session_id"] != turn["session_id"]:
                raise ValueError("feedback session is not bound to the native invocation")
            for hook in turn["hooks"]:
                identity = hook["id"]
                if type(identity) is not int or identity <= 0 or identity in seen:
                    raise ValueError("repeated or invalid hook identity")
                seen.add(identity)
                if (hook["session_id"] != turn["session_id"] or hook.get("agent") != metadata["agent"]
                        or str(Path(hook["path"]).resolve()) != str(Path(metadata["workspace"]).resolve())):
                    raise ValueError("feedback belongs to another Agent, session or workspace")
                if hook["state"] != "recorded" or hook["error"]:
                    unknown.append({"id": identity, "reason": "hook output was not recorded successfully"})
                    continue
                if "decision" not in hook:
                    unknown.append({"id": identity, "reason": "recorded decision was not collected"})
                    continue
                decision = json.loads(hook["decision"])
                if not isinstance(decision, dict) or set(decision) != DECISION_FIELDS or any(
                        not isinstance(value, str) for value in decision.values()):
                    raise ValueError("invalid recorded decision fields")
                inputs.append({"id": identity, "agent": hook["agent"], "kind": hook["kind"], "decision": decision})
        payload = json.dumps(inputs, ensure_ascii=False).encode()
        binary_hash = dataset.digest(renderer)
        native = subprocess.run([str(renderer)], input=payload, capture_output=True, timeout=30)
        if native.returncode or dataset.digest(renderer) != binary_hash:
            raise ValueError("product feedback renderer failed or changed")
        outputs = json.loads(native.stdout)
        by_id = {item["id"]: item for item in outputs}
        if len(by_id) != len(outputs) or set(by_id) != {item["id"] for item in inputs}:
            raise ValueError("rendered feedback does not match recorded hook identities")
        counts = Counter()
        records = []
        for item in outputs:
            if item["unknown"]:
                unknown.append({"id": item["id"], "reason": item["unknown"]})
                continue
            fields = item["fields"]
            if not isinstance(fields, list):
                raise ValueError("invalid rendered text fields")
            names = set()
            for field in fields:
                if field["name"] not in {"systemMessage", "reason", "additionalContext", "permissionDecisionReason"} or field["name"] in names:
                    raise ValueError("unexpected or repeated rendered text field")
                names.add(field["name"])
                for key in ("utf8_bytes", "characters", "estimated_tokens"):
                    if type(field[key]) is not int or field[key] < 0:
                        raise ValueError("invalid rendered feedback count")
                    counts[key] += field[key]
                counts["fields"] += 1
            counts["hooks_with_text"] += bool(fields)
            records.append(item)
        result.update(status="partial" if unknown else "recorded", unknown=unknown,
                      observed={"hooks": len(records), **{key: counts[key] for key in (
                          "hooks_with_text", "fields", "utf8_bytes", "characters", "estimated_tokens")}},
                      records=records, source={"renderer_sha256": binary_hash,
                          "input_sha256": hashlib.sha256(payload).hexdigest(),
                          "output_sha256": hashlib.sha256(native.stdout).hexdigest()})
    except (OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.SubprocessError) as error:
        result["unknown"].append({"reason": str(error)})
    return result
