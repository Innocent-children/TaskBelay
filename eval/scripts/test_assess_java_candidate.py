import json
import os
from pathlib import Path
import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET

import assess_candidate
import dataset


class JavaCandidateAssessmentTests(unittest.TestCase):
    def test_native_maven_preserves_tests_resources_workspace_and_fresh_reports(self):
        java_home = os.environ.get("TASKBELAY_EVAL_JAVA_HOME")
        maven = shutil.which("mvn")
        if not java_home or not (Path(java_home) / "bin/java").is_file() or not maven:
            self.skipTest("native Maven and TASKBELAY_EVAL_JAVA_HOME are required")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            source, task, agent = (root / name for name in ("source", "task", "agent"))
            for path in (source, task, agent):
                path.mkdir()
            dataset.git(source, "init", "--initial-branch=main")

            def commit():
                dataset.git(source, "add", "--all")
                dataset.git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                            "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
                return dataset.git(source, "rev-parse", "HEAD").stdout.decode().strip()

            (source / "pom.xml").write_text('''<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion><groupId>fixture</groupId><artifactId>candidate</artifactId><version>1</version>
  <properties><maven.compiler.release>17</maven.compiler.release><project.build.sourceEncoding>UTF-8</project.build.sourceEncoding></properties>
  <dependencies><dependency><groupId>org.junit.jupiter</groupId><artifactId>junit-jupiter</artifactId><version>5.14.4</version><scope>test</scope></dependency></dependencies>
  <build><plugins>
    <plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-compiler-plugin</artifactId><version>3.14.1</version></plugin>
    <plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-surefire-plugin</artifactId><version>3.5.5</version></plugin>
  </plugins></build>
</project>
''')
            (source / ".gitignore").write_text("/target/\n")
            for name in ("src/main/java", "src/test/java", "src/test/resources"):
                (source / name).mkdir(parents=True)
            value_path = "src/main/java/Value.java"
            original = ('import org.junit.jupiter.api.Test; import static org.junit.jupiter.api.Assertions.*;\n'
                        'class ValueTest { @Test void existing() throws Exception { assertTrue(Value.value() >= Fixture.threshold()); } }\n')
            helper = ('class Fixture { static int threshold() throws Exception { try (var in = Fixture.class.getResourceAsStream("/threshold.txt")) '
                      '{ return Integer.parseInt(new String(in.readAllBytes()).trim()); } } }\n')
            (source / value_path).write_text("class Value { static int value() { return 1; } }\n")
            (source / "src/test/java/ValueTest.java").write_text(original)
            (source / "src/test/java/Fixture.java").write_text(helper)
            (source / "src/test/java/SkippedTest.java").write_text(
                'import org.junit.jupiter.api.*; @Disabled("fixture") class SkippedTest { @Test void skip() {} }\n')
            (source / "src/test/resources/threshold.txt").write_text("0\n")
            base = commit()
            (source / value_path).write_text("class Value { static int value() { return 2; } }\n")
            (source / "src/test/java/HiddenTest.java").write_text(
                'import org.junit.jupiter.params.ParameterizedTest; import org.junit.jupiter.params.provider.ValueSource;\n'
                'import static org.junit.jupiter.api.Assertions.*; class HiddenTest {\n'
                '@ParameterizedTest @ValueSource(strings={"<a&b>", "中文"}) void expected(String label) { assertEquals(2, Value.value(), label); } }\n')
            merged = commit()
            for name, paths in (("reference.patch", []), ("implementation.patch", [value_path]),
                                ("hidden.patch", ["src/test/java/HiddenTest.java"])):
                (task / name).write_bytes(dataset.git(source, "diff", base, merged, "--", *paths).stdout)
            (task / "request.md").write_text("Return the requested value.\n")
            manifest = {"id": "java-fixture", "language": "Java", "base_commit": base, "merge_commit": merged,
                        "base_tree": dataset.git(source, "rev-parse", base + "^{tree}").stdout.decode().strip(),
                        "files": [{"path": value_path, "category": "source"}],
                        "sha256": {path.name: dataset.digest(path) for path in task.iterdir()},
                        "checks": {"existing": [[maven, "-B", "-ntp", "test"]],
                                   "hidden": [[maven, "-B", "-ntp", "-Dtest=HiddenTest", "test"]]}}
            (task / "task.json").write_text(json.dumps(manifest))
            workspace = root / "workspace"
            dataset.prepare(task, source, workspace)
            (agent / "metadata.json").write_text(json.dumps({"workspace": str(workspace), "task_id": manifest["id"]}))
            runtime = {"environment": {"JAVA_HOME": java_home, "PATH": str(Path(java_home) / "bin") + os.pathsep + os.environ["PATH"]},
                       "versions": [[maven, "-version"]], "timeout_seconds": 120}
            weak = "class ValueTest {}\n"
            for value, integrated in ((-1, True), (2, False)):
                (workspace / value_path).write_text(f"class Value {{ static int value() {{ return {value}; }} }}\n")
                (workspace / "src/test/java/ValueTest.java").write_text(weak)
                (workspace / "src/test/java/Fixture.java").write_text("class Fixture { static int threshold() { return -999; } }\n")
                (workspace / "src/test/resources/threshold.txt").write_text("-999\n")
                (workspace / "src/test/java/NewTest.java").write_text("new test must stay in candidate only\n")
                (agent / "result.json").write_text(json.dumps({"valid_integration": integrated, "turns": [{"returncode": 0}]}))
                files = assess_candidate.working_files(workspace)
                index = workspace / ".git/index"
                before_index = (index.read_bytes(), index.stat().st_mtime_ns)
                output = root / ("failed-behavior" if value == -1 else "missing-hooks")
                result = assess_candidate.assess(task, source, workspace, agent, output, runtime)
                self.assertNotIn("error", result)
                self.assertTrue(result["correctness_evaluated"], result)
                expected = "fail" if value == -1 else "pass"
                self.assertEqual((result["existing"]["status"], result["hidden"]["status"]), (expected, expected))
                self.assertEqual(result["valid_correctness_sample"], integrated)
                self.assertEqual(result["existing"]["test_result_events"], 1)
                self.assertEqual(len(result["existing"]["skipped"]), 1)
                self.assertEqual(result["hidden"]["test_result_events"], 2)
                self.assertEqual(assess_candidate.working_files(workspace), files)
                self.assertEqual((index.read_bytes(), index.stat().st_mtime_ns), before_index)
                self.assertFalse((workspace / "target").exists())
                self.assertFalse((workspace / "src/test/java/HiddenTest.java").exists())
                self.assertEqual((output / "candidate/src/test/java/ValueTest.java").read_text(), weak)
                self.assertTrue((output / "candidate/src/test/java/NewTest.java").exists())
                self.assertEqual((output / "validation/src/test/java/ValueTest.java").read_text(), original)
                self.assertEqual((output / "validation/src/test/java/Fixture.java").read_text(), helper)
                self.assertEqual((output / "validation/src/test/resources/threshold.txt").read_text(), "0\n")
                self.assertFalse((output / "validation/src/test/java/NewTest.java").exists())
                self.assertEqual(len(list((output / "hidden/1/previous-reports").glob("TEST-*.xml"))), 2)
                self.assertEqual(len(result["hidden"]["commands"][0]["test_reports"]), 1)

            (workspace / value_path).write_text("class Value { broken compilation }\n")
            broken = assess_candidate.assess(task, source, workspace, agent, root / "compilation", runtime)
            self.assertFalse(broken["correctness_evaluated"])
            self.assertEqual((broken["existing"]["status"], broken["hidden"]["status"]), ("unknown", "unknown"))
            (workspace / value_path).write_text("class Value { static int value() { return 2; } }\n")
            manifest["checks"]["hidden"][0].append("-DskipTests")
            (task / "task.json").write_text(json.dumps(manifest))
            stale = assess_candidate.assess(task, source, workspace, agent, root / "stale-reports", runtime)
            self.assertEqual((stale["existing"]["status"], stale["hidden"]["status"]), ("pass", "unknown"))
            self.assertEqual(stale["hidden"]["commands"][0]["exit_code"], 0)
            self.assertEqual(stale["hidden"]["commands"][0]["test_reports"], [])
            self.assertTrue(list((root / "stale-reports/hidden/1/previous-reports").glob("TEST-*.xml")))
            (workspace / "pom.xml").write_text((workspace / "pom.xml").read_text().replace("</properties>", "<skipTests>true</skipTests></properties>"))
            changed = assess_candidate.assess(task, source, workspace, agent, root / "configuration", runtime)
            self.assertEqual(changed["changed_test_configuration"], ["pom.xml"])
            self.assertFalse(changed["correctness_evaluated"])
            self.assertNotIn("commands", changed["existing"])

    def test_native_surefire_reports_require_complete_and_consistent_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            folder = root / "reports"
            folder.mkdir()

            def record(outcome="pass"):
                for path in folder.iterdir():
                    path.unlink()
                status = {"pass": None, "fail": "failure", "skip": "skipped", "error": "error"}[outcome]
                reports = []
                totals = [2, int(outcome == "fail"), int(outcome == "error"), int(outcome == "skip")]
                for index in range(2):
                    terminal = status if index else None
                    suite = ET.Element("testsuite", name=f"ValueTest{index}", tests="1", failures=str(int(terminal == "failure")),
                                       errors=str(int(terminal == "error")), skipped=str(int(terminal == "skipped")))
                    case = ET.SubElement(suite, "testcase", classname=f"ValueTest{index}", name="expected(String)[1]")
                    if terminal:
                        ET.SubElement(case, terminal, message="中文 <a&b>").text = "native detail"
                    path = folder / f"TEST-ValueTest{index}.xml"
                    ET.ElementTree(suite).write(path, encoding="utf-8", xml_declaration=True)
                    reports.append({"path": str(path.relative_to(root)), "sha256": dataset.digest(path)})
                failed = outcome in {"fail", "error"}
                (root / "stdout.log").write_text("[INFO] Tests run: %s, Failures: %s, Errors: %s, Skipped: %s\n" % tuple(totals)
                                               + ("[ERROR] BUILD FAILURE" if failed else "[INFO] BUILD SUCCESS") + "\n")
                (root / "stderr.log").write_text("")
                return {"test_report_directory": "reports", "test_reports": reports, "test_report_diagnostics": [],
                        "stdout": "stdout.log", "stderr": "stderr.log", "stdout_sha256": dataset.digest(root / "stdout.log"),
                        "stderr_sha256": dataset.digest(root / "stderr.log"), "exit_code": int(failed), "timed_out": False}

            def status(value):
                return assess_candidate.test_status([value], root, "Java")

            for outcome in ("pass", "fail", "error", "skip"):
                result = status(record(outcome))
                self.assertEqual(result["status"], "fail" if outcome == "error" else "pass" if outcome == "skip" else outcome)
                self.assertEqual(result["test_result_events"], 1 if outcome == "skip" else 2)
            value = record("fail"); value["timed_out"] = True
            self.assertEqual(status(value)["status"], "unknown")
            value = record(); value["exit_code"] = 1
            self.assertEqual(status(value)["status"], "unknown")
            value = record("fail"); value["exit_code"] = 0
            self.assertEqual(status(value)["status"], "unknown")
            value = record(); (folder / "extra.dumpstream").write_text("fork terminated")
            self.assertEqual(status(value)["status"], "unknown")
            value = record(); shutil.copy2(folder / "TEST-ValueTest0.xml", folder / "TEST-extra.xml")
            self.assertEqual(status(value)["status"], "unknown")
            value = record(); (folder / "TEST-ValueTest0.xml").write_text("changed")
            self.assertEqual(status(value)["status"], "unknown")
            value = record(); value["test_reports"] = value["test_reports"][:1]
            self.assertEqual(status(value)["status"], "unknown")
            value = record(); (root / "stdout.log").write_text("[INFO] Tests run: 99, Failures: 0, Errors: 0, Skipped: 0\n[INFO] BUILD SUCCESS\n")
            value["stdout_sha256"] = dataset.digest(root / "stdout.log")
            self.assertEqual(status(value)["status"], "unknown")
            value = record(); path = folder / "TEST-ValueTest1.xml"
            suite = ET.parse(path); ET.SubElement(suite.find("testcase"), "flakyFailure").text = "first attempt failed"
            suite.write(path); value["test_reports"][1]["sha256"] = dataset.digest(path)
            self.assertEqual(status(value)["status"], "unknown")


if __name__ == "__main__":
    unittest.main()
