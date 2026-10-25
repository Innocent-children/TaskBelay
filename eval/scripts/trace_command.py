"""Forward a selected evaluation tool and record actual process starts and exits."""

import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import uuid

try:
    import resource
except ImportError:
    resource = None


def append_event(path, event):
    data = (json.dumps(event, ensure_ascii=False, separators=(",", ":")) + "\n").encode("utf-8")
    descriptor = os.open(path, os.O_WRONLY | os.O_APPEND | os.O_CREAT, 0o600)
    try:
        if os.write(descriptor, data) != len(data):
            raise OSError("incomplete command trace write")
    finally:
        os.close(descriptor)


def forward(config, arguments):
    # Python virtual environments select their prefix from the invoked entry
    # path. Resolving its symlink changes the interpreter environment.
    command = Path(config["command"])
    trace = Path(config["trace"]).resolve()
    if not command.is_absolute() or not command.is_file() or not trace.parent.is_dir():
        raise ValueError("an existing absolute native tool and trace directory are required")
    origin = os.environ.get("TASKBELAY_EVAL_TRACE_ORIGIN", "agent")
    if origin not in {"agent", "taskbelay", "assessment"}:
        raise ValueError("unknown evaluation trace origin")
    role = config.get("role", "native")
    if role not in {"native", "shell_invocation"}:
        raise ValueError("unknown evaluation trace role")
    identity = uuid.uuid4().hex
    parent = os.environ.get("TASKBELAY_EVAL_TRACE_PARENT")
    common = {"id": identity, "parent_id": parent, "origin": origin, "role": role, "wrapper_pid": os.getpid(),
              "cwd": str(Path.cwd()), "argv": [str(command), *arguments]}
    # Record the attempt before launching. An attempt without a start is not an
    # execution; a start without an exit is incomplete evidence, not a success.
    append_event(trace, {**common, "event": "attempt", "unix": time.time()})
    started = time.monotonic()
    usage_before = resource.getrusage(resource.RUSAGE_CHILDREN) if resource else None
    try:
        process = subprocess.Popen([str(command), *arguments], env={
            **os.environ, "TASKBELAY_EVAL_TRACE_PARENT": identity})
    except (OSError, ValueError) as error:
        append_event(trace, {"event": "launch_failed", "id": identity, "error": str(error), "unix": time.time()})
        raise
    termination_signal = None

    def terminate(signum, _frame):
        nonlocal termination_signal
        termination_signal = signum
        # An Agent may terminate the wrapper PID instead of its process group.
        # Forward cancellation to the actual tool and keep waiting to reap it.
        try:
            if os.name == "nt" and signum == signal.SIGINT:
                process.terminate()
            else:
                process.send_signal(signum)
        except ProcessLookupError:
            pass

    previous = {signum: signal.signal(signum, terminate) for signum in (signal.SIGTERM, signal.SIGINT)}
    try:
        trace_error = None
        try:
            append_event(trace, {"event": "start", "id": identity, "pid": process.pid, "unix": time.time()})
        except OSError as error:
            # A trace write failure must not abandon the native child.
            trace_error = error
        code = process.wait()
        usage_after = resource.getrusage(resource.RUSAGE_CHILDREN) if resource else None
        usage = None
        if usage_after is not None:
            usage = {"user_seconds": usage_after.ru_utime - usage_before.ru_utime,
                     "system_seconds": usage_after.ru_stime - usage_before.ru_stime,
                     "reported_maxrss": usage_after.ru_maxrss,
                     "maxrss_unit": "bytes" if sys.platform == "darwin" else "KiB"}
        append_event(trace, {"event": "exit", "id": identity, "returncode": code,
                             "elapsed_seconds": time.monotonic() - started, "resources": usage,
                             "termination_signal": termination_signal, "unix": time.time()})
        if trace_error is not None:
            raise trace_error
        if termination_signal is not None:
            return 128 + termination_signal
        return code if code >= 0 else 128 - code
    finally:
        for signum, handler in previous.items():
            signal.signal(signum, handler)


def collect(path):
    calls = {}
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        event = json.loads(line)
        kind, identity = event["event"], event["id"]
        if kind == "attempt":
            if identity in calls:
                raise ValueError("duplicate trace identity at line " + str(number))
            calls[identity] = {"attempt": event, "start": None, "exit": None, "launch_failed": None}
            continue
        if identity not in calls or kind not in {"start", "exit", "launch_failed"} or calls[identity][kind] is not None:
            raise ValueError("unordered or repeated command trace at line " + str(number))
        call = calls[identity]
        if (kind == "exit" and call["start"] is None) or (kind == "start" and call["launch_failed"] is not None):
            raise ValueError("command trace has no valid start at line " + str(number))
        if kind == "launch_failed" and call["start"] is not None:
            raise ValueError("started command was reported as not launched")
        call[kind] = event
    result = []
    for identity, call in calls.items():
        parent = call["attempt"]["parent_id"]
        if parent and parent not in calls:
            raise ValueError("trace parent is missing: " + parent)
        state = "not_started" if call["launch_failed"] else "complete" if call["exit"] else "incomplete"
        result.append({"id": identity, **call, "state": state})
    return {"calls": result, "observed_starts": sum(call["start"] is not None for call in calls.values()),
            "complete": bool(result) and all(call["state"] != "incomplete" for call in result),
            "scope": "selected forwarded commands only; nested calls are linked, unobserved processes remain unknown"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("config", type=Path)
    parser.add_argument("arguments", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    arguments = args.arguments[1:] if args.arguments[:1] == ["--"] else args.arguments
    raise SystemExit(forward(json.loads(args.config.read_text()), arguments))


if __name__ == "__main__":
    main()
