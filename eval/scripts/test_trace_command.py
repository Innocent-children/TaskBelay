import json
import os
import signal
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
import venv
from unittest import mock

import trace_command


class CommandTraceTests(unittest.TestCase):
    @unittest.skipIf(os.name == "nt", "POSIX virtual environment symlink fixture")
    def test_interpreter_symlink_keeps_its_virtual_environment(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            environment = root / "venv"
            venv.EnvBuilder(with_pip=False, symlinks=True).create(environment)
            command = environment / "bin/python"
            self.assertTrue(command.is_symlink())
            config, trace = root / "config.json", root / "trace.jsonl"
            config.write_text(json.dumps({"command": str(command), "trace": str(trace)}))
            program = "import sys; from pathlib import Path; print(sys.prefix); sys.exit(0 if Path(sys.prefix).resolve()==Path(sys.argv[1]) else 23)"
            result = subprocess.run([sys.executable, str(Path(trace_command.__file__).resolve()), str(config),
                                     "--", "-c", program, str(environment)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(Path(result.stdout.strip()).resolve(), environment)
            report = trace_command.collect(trace)
            self.assertTrue(report["complete"])
            self.assertEqual(report["calls"][0]["attempt"]["argv"][0], str(command))

    @unittest.skipIf(os.name == "nt", "native Windows cancellation remains pending")
    def test_termination_reaches_native_child_and_is_not_reported_as_success(self):
        for signum in (signal.SIGTERM, signal.SIGINT):
            with self.subTest(signal=signum):
                self.check_termination(signum)

    def check_termination(self, signum):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ready, stopped = root / "ready", root / "stopped"
            program = ("import signal,time,sys\nfrom pathlib import Path\n"
                       "def stop(signum, frame):\n"
                       f" Path({str(stopped)!r}).write_text(str(signum))\n"
                       " sys.exit(0)\n"
                       "signal.signal(signal.SIGTERM,stop)\n"
                       "signal.signal(signal.SIGINT,stop)\n"
                       f"Path({str(ready)!r}).touch()\n"
                       "while True: time.sleep(1)\n")
            config = root / "native.json"
            trace = root / "trace.jsonl"
            config.write_text(json.dumps({"command": sys.executable, "trace": str(trace)}))
            process = subprocess.Popen([sys.executable, str(Path(trace_command.__file__).resolve()), str(config),
                                        "--", "-c", program], start_new_session=True,
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                deadline = time.monotonic() + 5
                while not ready.exists() and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue(ready.exists(), "native process did not become ready")
                process.send_signal(signum)
                code = process.wait(timeout=5)
                report = trace_command.collect(trace)
                self.assertTrue(report["complete"])
                self.assertEqual(stopped.read_text(), str(signum))
                self.assertEqual(code, 128 + signum)
                self.assertEqual(report["calls"][0]["exit"]["termination_signal"], signum)
                self.assertEqual(report["calls"][0]["exit"]["returncode"], 0)
            finally:
                # This group was created above solely for this fixture. Reap its
                # child even when the old forwarding implementation dies first.
                try:
                    os.killpg(process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                process.wait(timeout=5)

    def test_trace_write_failure_does_not_abandon_started_native_child(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            marker = root / "child-finished"
            append = trace_command.append_event

            def fail_start(path, event):
                if event["event"] == "start":
                    raise OSError("trace storage unavailable")
                append(path, event)

            with mock.patch.object(trace_command, "append_event", side_effect=fail_start):
                with self.assertRaisesRegex(OSError, "trace storage unavailable"):
                    trace_command.forward({"command": sys.executable, "trace": str(root / "trace.jsonl")},
                        ["-c", f"from pathlib import Path; Path({str(marker)!r}).write_text('finished')"])
            self.assertEqual(marker.read_text(), "finished")
            with self.assertRaisesRegex(ValueError, "valid start"):
                trace_command.collect(root / "trace.jsonl")

    def test_native_output_exit_and_nested_calls_are_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            script = Path(trace_command.__file__).resolve()
            config = root / "tool.json"
            trace = root / "trace.jsonl"
            config.write_text(json.dumps({"command": sys.executable, "trace": str(trace)}))
            invocation = [sys.executable, str(script), str(config), "--"]
            parent_config = root / "shell.json"
            parent_config.write_text(json.dumps({"command": sys.executable, "trace": str(trace),
                                                  "role": "shell_invocation"}))
            child = [*invocation, "-c", "import sys; print(sys.argv[1]); sys.exit(7)", "literal $HOME `text` 中文"]
            program = f"import subprocess,sys; p=subprocess.run({child!r}); print('parent',flush=True); sys.exit(p.returncode)"
            run = subprocess.run([sys.executable, str(script), str(parent_config), "--", "-c", program],
                                 cwd=root, capture_output=True, text=True,
                                 env={**os.environ, "TASKBELAY_EVAL_TRACE_ORIGIN": "assessment"})
            self.assertEqual(run.returncode, 7)
            self.assertEqual(run.stdout, "literal $HOME `text` 中文\nparent\n")
            self.assertEqual(run.stderr, "")
            report = trace_command.collect(trace)
            self.assertTrue(report["complete"])
            self.assertEqual(report["observed_starts"], 2)
            parent, nested = report["calls"]
            self.assertEqual(parent["attempt"]["role"], "shell_invocation")
            self.assertEqual(nested["attempt"]["role"], "native")
            self.assertIsNone(parent["attempt"]["parent_id"])
            self.assertEqual(nested["attempt"]["parent_id"], parent["id"])
            self.assertEqual(nested["attempt"]["argv"][-1], "literal $HOME `text` 中文")
            for call in report["calls"]:
                self.assertEqual(call["attempt"]["origin"], "assessment")
                self.assertEqual(call["exit"]["returncode"], 7)
                self.assertGreaterEqual(call["exit"]["elapsed_seconds"], 0)

    def test_failed_launch_interruption_and_invalid_trace_never_count_as_success(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / "trace.jsonl"
            attempt = {"event": "attempt", "id": "one", "parent_id": None}
            trace_command.append_event(path, attempt)
            self.assertFalse(trace_command.collect(path)["complete"])
            self.assertEqual(trace_command.collect(path)["observed_starts"], 0)
            trace_command.append_event(path, {"event": "start", "id": "one", "pid": 10})
            report = trace_command.collect(path)
            self.assertFalse(report["complete"])
            self.assertEqual(report["observed_starts"], 1)
            trace_command.append_event(path, {"event": "launch_failed", "id": "one"})
            with self.assertRaises(ValueError):
                trace_command.collect(path)
            path.write_text("")
            trace_command.append_event(path, attempt)
            trace_command.append_event(path, {"event": "launch_failed", "id": "one", "error": "missing tool"})
            report = trace_command.collect(path)
            self.assertTrue(report["complete"])
            self.assertEqual(report["observed_starts"], 0)
            self.assertEqual(report["calls"][0]["state"], "not_started")
            trace_command.append_event(path, attempt)
            with self.assertRaises(ValueError):
                trace_command.collect(path)


if __name__ == "__main__":
    unittest.main()
