package codescan

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestDocumentedDirectDependencyManifests(t *testing.T) {
	tests := []struct {
		file, source string
		want         []string
		lock         string
	}{
		{"sub/go.mod", "module example.com/app\ngo 1.26\nrequire (\n example.com/direct v1.0.0\n example.com/transitive v1.0.0 // indirect\n)\nrequire example.com/other v2.0.0\nreplace example.com/replace => ../local\n", []string{"example.com/direct", "example.com/other"}, "sub/go.sum"},
		{"package.json", `{"dependencies":{"a":"1"},"devDependencies":{"b":"2"},"peerDependencies":{"a":"*","c":"3"},"optionalDependencies":{"d":"4"},"scripts":{"build":"unused"}}`, []string{"a", "b", "c", "d"}, "pnpm-lock.yaml"},
		{"pyproject.toml", `[project]
dependencies = ["requests>=2", "rich[all] ; python_version >= '3.8'"]
[project.optional-dependencies]
test = ["pytest", "coverage"]
[dependency-groups]
dev = ["ruff", {include-group = "test"}]
[tool.poetry.dependencies]
python = "^3.12"
PyYAML = {version="6"}
[tool.poetry.group.test.dependencies]
pytest = "*"
[tool.poetry.dev-dependencies]
black = "*"
[build-system]
requires = ["not-in-documented-groups"]
`, []string{"PyYAML", "black", "coverage", "pytest", "requests", "rich", "ruff"}, "uv.lock"},
		{"requirements-dev.txt", "# comment\n-r base.txt\n-e ./local\nRequests[security]>=2 # note\nnamed @ https://example.test/source.whl\n", []string{"Requests", "named"}, ""},
		{"Cargo.toml", `[dependencies]
serde = {version="1",features=["derive"]}
[dev-dependencies]
pretty_assertions = "1"
[build-dependencies]
cc = "1"
[target.'cfg(unix)'.dependencies]
libc = "1"
[workspace.dependencies]
tokio = "1"
[dependencies.renamed]
package = "underlying-name"
version = "1"
[patch.crates-io]
not_direct = {path="../local"}
`, []string{"cc", "libc", "pretty_assertions", "renamed", "serde", "tokio"}, "Cargo.lock"},
		{"pom.xml", `<project xmlns="http://maven.apache.org/POM/4.0.0"><dependencyManagement><dependencies><dependency><groupId>managed</groupId><artifactId>unused</artifactId></dependency></dependencies></dependencyManagement><dependencies><dependency><groupId>org.demo</groupId><artifactId>lib</artifactId></dependency></dependencies><build><plugins><plugin><dependencies><dependency><groupId>plugin</groupId><artifactId>internal</artifactId></dependency></dependencies></plugin></plugins></build></project>`, []string{"org.demo:lib"}, ""},
		{"build.gradle", "dependencies { implementation 'g:one:1'\n testImplementation group: 'g', name: 'two', version: '2'\n// api 'ignored:a:1'\n/* runtimeOnly 'ignored:b:1' */\n compileOnly \"g:three:3\" }\n def example = \"api 'ignored:c:1'\"", []string{"g:one", "g:three", "g:two"}, "gradle.lockfile"},
		{"build.gradle.kts", "dependencies { api(\"g:one:1\"); testRuntimeOnly(\"g:two:2\"); implementation(platform(\"g:bom:1\")); implementation(project(\":local\")) }", []string{"g:bom", "g:one", "g:two"}, "gradle.lockfile"},
		{"Gemfile", "source 'https://rubygems.org'\ngem 'rack', '~>3'\ngroup :test do\n gem(\"rspec\")\nend\n# gem 'ignored'\ntext=\"gem 'not-real'\"\n", []string{"rack", "rspec"}, "Gemfile.lock"},
		{"composer.json", `{"require":{"vendor/package":"1","php":"8"},"require-dev":{"vendor/test":"2"},"suggest":{"ignored":"*"}}`, []string{"php", "vendor/package", "vendor/test"}, "composer.lock"},
		{"app.csproj", `<Project><ItemGroup><PackageReference Include="Newtonsoft.Json" Version="1"/><PackageReference Include="Test.Library"><Version>2</Version></PackageReference><PackageReference Update="Not.New" Version="3"/></ItemGroup></Project>`, []string{"Newtonsoft.Json", "Test.Library"}, "packages.lock.json"},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			items, err := Dependencies(t.Context(), test.file, []byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, item := range items {
				got = append(got, item.Name)
				if item.Manifest != test.file {
					t.Fatal(item)
				}
				if test.lock != "" && !strings.Contains(strings.Join(item.Locks, "|"), test.lock) {
					t.Fatal("missing associated lock", item)
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v; want %v", got, test.want)
			}
		})
	}
}

func TestDependenciesPreserveUnknownAndOnlyReportAddedNames(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{"go.mod", "require (\nfoo v1\n"},
		{"package.json", `{"dependencies":[]}`},
		{"pyproject.toml", "[project\ndependencies = [\"bad\"]"},
		{"pom.xml", "<project><dependencies>"},
		{"build.gradle", `implementation(libs.dynamic)`},
		{"Gemfile", `gem ENV['LIBRARY']`},
		{"requirements.txt", "https://example.test/archive.whl"},
	} {
		if _, err := Dependencies(t.Context(), test.name, []byte(test.source)); err == nil {
			t.Error("unknown manifest accepted", test.name)
		}
	}
	before, _ := Dependencies(t.Context(), "package.json", []byte(`{"dependencies":{"a":"1"}}`))
	after, _ := Dependencies(t.Context(), "package.json", []byte(`{"dependencies":{"a":"2","b":"1"}}`))
	added := AddedDependencies(before, after)
	if len(added) != 1 || added[0].Name != "b" {
		t.Fatal(added)
	}
	partial, err := Dependencies(t.Context(), "build.gradle", []byte("implementation 'g:known:1'\napi(libs.unknown)"))
	if err == nil || len(partial) != 1 || partial[0].Name != "g:known" {
		t.Fatal("known facts or unknown scope lost", partial, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Dependencies(ctx, "Cargo.toml", []byte("[dependencies]")); err != context.Canceled {
		t.Fatal(err)
	}
}
