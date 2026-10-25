package testrun

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestBuildCoverageRequiresFixedSourceAndValidRecords(t *testing.T) {
	executor, request := nativeToolRequest(t, map[string]string{"src/main/java/example/Limit.java": "package example;\nclass Limit {int value(){return 1;}}\n", "src/lib.rs": "pub fn value()->i32{1}\n"}, t.TempDir())
	profile := filepath.Join(executor.Output, "coverage")
	for _, tc := range []struct {
		name, data  string
		java, valid bool
	}{
		{"java-valid", `<report><package name="example"><sourcefile name="Limit.java"><line nr="2" ci="1" mi="0"/></sourcefile></package></report>`, true, true},
		{"java-outside", `<report><package name="../outside"><sourcefile name="Limit.java"><line nr="2" ci="1" mi="0"/></sourcefile></package></report>`, true, false},
		{"java-line", `<report><package name="example"><sourcefile name="Limit.java"><line nr="20" ci="1" mi="0"/></sourcefile></package></report>`, true, false},
		{"java-wrong-report", `<testsuite/>`, true, false},
		{"rust-valid", "SF:" + filepath.Join(executor.Root, "src/lib.rs") + "\nDA:1,1\nend_of_record\n", false, true},
		{"rust-truncated", "SF:" + filepath.Join(executor.Root, "src/lib.rs") + "\nDA:1,1\n", false, false},
		{"rust-invalid-count", "SF:" + filepath.Join(executor.Root, "src/lib.rs") + "\nDA:1,-1\nend_of_record\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(profile, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			var coverage Coverage
			if tc.java {
				coverage = readJaCoCo(t.Context(), profile, filepath.Join(executor.Root, "src/main/java"), executor.Root, request.Snapshot)
			} else {
				coverage = readLCOV(t.Context(), executor.Root, executor.Workspace, profile, request.Snapshot)
			}
			if coverage.Valid != tc.valid || tc.valid && !coverage.CopyExecuted {
				t.Fatal(coverage)
			}
		})
	}
	if err := os.MkdirAll(filepath.Join(executor.Workspace, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(executor.Workspace, "src/lib.rs"), []byte("pub fn value()->i32{1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, []byte("SF:"+filepath.Join(executor.Workspace, "src/lib.rs")+"\nDA:1,1\nend_of_record\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if coverage := readLCOV(t.Context(), executor.Root, executor.Workspace, profile, request.Snapshot); coverage.Valid || !strings.Contains(coverage.Reason, "原工作区") {
		t.Fatal(coverage)
	}
}

func TestJavaGradleCacheAgentAndWrapperIsolation(t *testing.T) {
	cache, out, root := t.TempDir(), t.TempDir(), t.TempDir()
	jar := filepath.Join(cache, "caches/modules-2/files-2.1/org.jacoco/org.jacoco.agent/0.8.15/hash/org.jacoco.agent-0.8.15.jar")
	if err := os.MkdirAll(filepath.Dir(jar), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(jar)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("jacocoagent.jar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte("cached-agent")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	path, version, err := cachedJaCoCoAgent([]string{cache}, out)
	if err != nil || version != "0.8.15" || path != filepath.Join(out, "jacocoagent.jar") {
		t.Fatal(path, version, err)
	}
	address := "https://services.gradle.org/distributions/gradle-9.8.2-bin.zip"
	property := filepath.Join(root, "gradle/wrapper/gradle-wrapper.properties")
	if err := os.MkdirAll(filepath.Dir(property), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(property, []byte("distributionUrl="+address+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(property), "gradle-wrapper.jar"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	command := javaCommand{tool: "gradle", prefix: []string{filepath.Join(root, "gradlew")}}
	executor := &Executor{Root: root, Output: out}
	if err := command.prepareWrapper(t.Context(), executor, []string{cache}, filepath.Join(out, "gradle-home")); err == nil {
		t.Fatal("missing distribution must not launch wrapper")
	}
	sum := md5.Sum([]byte(address))
	relative := filepath.Join("wrapper/dists/gradle-9.8.2-bin", new(big.Int).SetBytes(sum[:]).Text(36))
	binary := filepath.Join(cache, relative, "gradle-9.8.2/bin/gradle")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, relative, "gradle-9.8.2-bin.zip.ok"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(out, "gradle-home")
	if err := command.prepareWrapper(t.Context(), executor, []string{cache}, home); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(command.env, "GRADLE_USER_HOME="+home) {
		t.Fatal(command.env)
	}
	copyPath := filepath.Join(home, relative, "gradle-9.8.2/bin/gradle")
	info, err := os.Stat(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0100 == 0 {
		t.Fatal("wrapper binary lost executable mode")
	}
	if err := os.WriteFile(copyPath, []byte("changed-copy"), 0600); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(binary)
	if string(original) != "fixture" {
		t.Fatal("original cache changed")
	}
}

func TestJavaSelectionRejectsOtherMethodsAndMissingEvents(t *testing.T) {
	dir := t.TempDir()
	for _, data := range []string{`{"Class":"example.Test","Name":"other()","Parents":[]}`, `{"Class":"other.Test","Name":"rows()[1]","Parents":["rows()"]}`, ""} {
		if err := os.WriteFile(filepath.Join(dir, "selected.jsonl"), []byte(data+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := checkGradleSelection(dir, "example.Test#rows", 1); err == nil {
			t.Fatal(data)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "selected.jsonl"), []byte("{\"Class\":\"example.Test\",\"Name\":\"[1] 2\",\"Parents\":[\"rows(int)\"]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkGradleSelection(dir, "example.Test#rows", 1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := copyJavaCache(ctx, dir, t.TempDir()); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestRustIntegrationTargetsKeepTheirOwnTestNames(t *testing.T) {
	runner := &RustRunner{targets: []rustTarget{{Key: "one", Directory: ".", Kind: "test", Source: "tests/one.rs"}, {Key: "two", Directory: ".", Kind: "test", Source: "tests/two.rs"}}, names: map[string][]string{"one": {"same"}, "two": {"same"}}}
	_, request := nativeToolRequest(t, map[string]string{"tests/one.rs": "#[test] fn same(){assert!(true);}", "tests/two.rs": "#[test] fn same(){assert!(true);}"}, t.TempDir())
	cases, result := runner.Enumerate(t.Context(), request.Cases)
	if !result.Passed() || len(cases) != 2 {
		t.Fatal(result, cases)
	}
	for _, test := range cases {
		want := strings.TrimSuffix(strings.TrimPrefix(test.Source.File, "tests/"), ".rs")
		if test.Package != want {
			t.Fatal(fmt.Sprint(test))
		}
	}
}

func TestBuildCommandEnvironmentCannotOverrideTemporaryOutputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX env 字面语法")
	}
	root := t.TempDir()
	java, err := parseJavaCommand("env JAVA_HOME='/project jdk' GRADLE_USER_HOME=/project-cache gradle test", root)
	if err != nil || !slices.Equal(java.prefix, []string{"env", "gradle"}) || !slices.Contains(java.env, "JAVA_HOME=/project jdk") || !slices.Contains(java.env, "GRADLE_USER_HOME=/project-cache") {
		t.Fatal(java, err)
	}
	cargo, err := parseCargoCommand("env CARGO_TARGET_DIR=/old CARGO_HOME=/dependencies cargo test", root)
	if err != nil || !slices.Equal(cargo.prefix, []string{"env", "cargo"}) || !slices.Contains(cargo.env, "CARGO_HOME=/dependencies") {
		t.Fatal(cargo, err)
	}
	cargo.env = mergeEnvironment(cargo.env, []string{"CARGO_TARGET_DIR=/temporary"})
	if slices.Contains(cargo.env, "CARGO_TARGET_DIR=/old") || !slices.Contains(cargo.env, "CARGO_TARGET_DIR=/temporary") {
		t.Fatal(cargo.env)
	}
}

func TestMavenOnlyScriptWrapperCacheIsCopiedBeforeExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX only-script wrapper 样例")
	}
	root, cache, out := t.TempDir(), t.TempDir(), t.TempDir()
	address := "https://repo.maven.apache.org/maven2/org/apache/maven/apache-maven/3.10.0/apache-maven-3.10.0-bin.zip"
	program := filepath.Join(root, "mvnw")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nhash_string() { :; }\n"), 0700); err != nil {
		t.Fatal(err)
	}
	property := filepath.Join(root, ".mvn/wrapper/maven-wrapper.properties")
	if err := os.MkdirAll(filepath.Dir(property), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(property, []byte("distributionUrl="+address+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command, err := parseJavaCommand("mvn test", root)
	if err != nil || command.prefix[len(command.prefix)-1] != program {
		t.Fatal(command, err)
	}
	executor := &Executor{Root: root, Output: out}
	if err := command.prepareWrapper(t.Context(), executor, []string{cache}, out); err == nil {
		t.Fatal("uncached wrapper must remain incomplete")
	}
	var value uint32
	for _, b := range []byte(address) {
		value = value*31 + uint32(b)
	}
	relative := filepath.Join("wrapper/dists/apache-maven-3.10.0", fmt.Sprintf("%x", value), "bin/mvn")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(cache, relative)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, relative), []byte("existing-maven"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := command.prepareWrapper(t.Context(), executor, []string{cache}, out); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(command.env, "MAVEN_USER_HOME="+out) {
		t.Fatal(command.env)
	}
	if err := os.WriteFile(filepath.Join(out, relative), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cache, relative))
	if err != nil || string(data) != "existing-maven" {
		t.Fatal(string(data), err)
	}
}
