package testrun

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type mavenProject struct {
	Group        string   `xml:"groupId"`
	Artifact     string   `xml:"artifactId"`
	Packaging    string   `xml:"packaging"`
	Modules      []string `xml:"modules>module"`
	Dependencies []struct {
		Group    string `xml:"groupId"`
		Artifact string `xml:"artifactId"`
	} `xml:"dependencies>dependency"`
	Build struct {
		Directory   string `xml:"directory"`
		Source      string `xml:"sourceDirectory"`
		TestSource  string `xml:"testSourceDirectory"`
		Classes     string `xml:"outputDirectory"`
		TestClasses string `xml:"testOutputDirectory"`
		Plugins     []struct {
			Artifact string `xml:"artifactId"`
			Version  string `xml:"version"`
			Config   struct {
				ArgLine   string   `xml:"argLine"`
				Reports   string   `xml:"reportsDirectory"`
				Fork      string   `xml:"forkCount"`
				JVM       string   `xml:"jvm"`
				Classpath []string `xml:"additionalClasspathElements>additionalClasspathElement"`
			} `xml:"configuration"`
		} `xml:"plugins>plugin"`
	} `xml:"build"`
}

func mavenModels(ctx context.Context, executor *Executor, command javaCommand, caches []string) ([]mavenProject, Result) {
	_, version := cachedJavaArtifact(caches, "org.apache.maven.plugins", "maven-help-plugin", "*", "")
	if version == "" {
		return nil, failure(EnvironmentFailure, "本机 Maven 仓库缺少 effective-pom 所需插件")
	}
	path := filepath.Join(executor.Output, "effective-pom.xml")
	run := executor.Execute(ctx, "prepare", executor.Root, command.args("org.apache.maven.plugins:maven-help-plugin:"+version+":effective-pom", "-Doutput="+path), command.env)
	if !run.Passed() {
		return nil, run
	}
	var projects struct {
		XMLName  xml.Name
		Projects []mavenProject `xml:"project"`
	}
	if err := readXML(path, &projects); err != nil {
		return nil, failure(EnvironmentFailure, err.Error())
	}
	if projects.XMLName.Local == "project" {
		var single mavenProject
		if err := readXML(path, &single); err != nil {
			return nil, failure(EnvironmentFailure, err.Error())
		}
		projects.Projects = []mavenProject{single}
	}
	if len(projects.Projects) == 0 {
		return nil, failure(EnvironmentFailure, "Maven 未返回有效项目")
	}
	return projects.Projects, Result{}
}

func (r *JavaRunner) prepareMaven(ctx context.Context) Result {
	if err := r.command.prepareWrapper(ctx, r.exec, r.caches, filepath.Join(r.exec.Output, "maven-home")); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	version := r.exec.Execute(ctx, "prepare", r.exec.Root, r.command.args("--version"), r.command.env)
	if !version.Passed() {
		return version
	}
	for _, line := range strings.Split(version.Output, "\n") {
		if _, value, ok := strings.Cut(line, "runtime: "); ok {
			r.javaHome = strings.TrimSpace(value)
		}
		if value, ok := strings.CutPrefix(line, "Java home: "); ok {
			r.javaHome = strings.TrimSpace(value)
		}
	}
	models, result := mavenModels(ctx, r.exec, r.command, r.caches)
	if !result.Passed() {
		return result
	}
	for _, model := range models {
		if model.Packaging == "pom" {
			continue
		}
		module := javaModule{ID: model.Group + ":" + model.Artifact, Source: model.Build.Source, TestSource: model.Build.TestSource, Classes: model.Build.Classes, TestClasses: model.Build.TestClasses, Reports: filepath.Join(model.Build.Directory, "surefire-reports"), Build: model.Build.Directory}
		for _, plugin := range model.Build.Plugins {
			if plugin.Artifact != "maven-surefire-plugin" {
				continue
			}
			module.ArgLine = plugin.Config.ArgLine
			if plugin.Config.Reports != "" {
				module.Reports = plugin.Config.Reports
			}
			if plugin.Config.Fork == "0" || plugin.Config.JVM != "" || len(plugin.Config.Classpath) > 0 || strings.ContainsAny(module.ArgLine, "${@") {
				return failure(EnvironmentFailure, "Maven 测试 JVM、动态 argLine 或附加 classpath 尚不能核对")
			}
		}
		for _, path := range []string{module.Source, module.TestSource, module.Classes, module.TestClasses, module.Build, module.Reports} {
			if !r.inside(path) {
				return failure(EnvironmentFailure, "Maven 源码或输出越出固定副本: "+path)
			}
		}
		r.modules = append(r.modules, module)
	}
	if len(r.modules) == 0 {
		return failure(SelectionFailure, "Maven 没有可核对的 Java 模块")
	}
	return Result{}
}

func (r *JavaRunner) mavenCompile(ctx context.Context) Result {
	return r.exec.Execute(ctx, "compile", r.exec.Root, r.command.args("test-compile", "-DskipTests"), r.command.env)
}

func (r *JavaRunner) mavenRun(ctx context.Context, module javaModule, selector, profile, stage string) Result {
	for _, current := range r.modules {
		if !r.inside(current.Reports) {
			return failure(EnvironmentFailure, "Maven 报告位置已离开固定副本")
		}
		for path := range r.request.Snapshot.Files {
			rel, err := filepath.Rel(current.Reports, filepath.Join(r.exec.Root, path))
			if err == nil && filepath.IsLocal(rel) {
				return failure(EnvironmentFailure, "Maven 报告目录包含固定输入，不能清理")
			}
		}
		// 只清理固定副本中不包含输入文件的旧报告，防止上游模块或空选择复用旧结果。
		if err := os.RemoveAll(current.Reports); err != nil {
			return failure(EnvironmentFailure, err.Error())
		}
	}
	agent := "-javaagent:" + r.agent + "=destfile=" + profile + ",append=false"
	if strings.ContainsAny(r.agent+profile, ",=\"\n\r") {
		return failure(EnvironmentFailure, "JaCoCo agent 路径含尚不能核对的分隔符")
	}
	agent = "\"" + agent + "\""
	args := []string{"test", "-pl", module.ID, "-am", "-Dtest=" + selector, "-Dsurefire.failIfNoSpecifiedTests=false", "-DargLine=" + strings.TrimSpace(module.ArgLine+" "+agent), "-Djacoco.skip=true"}
	return r.exec.Execute(ctx, stage, r.exec.Root, r.command.args(args...), r.command.env)
}

func (r *JavaRunner) inside(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(r.exec.Root, path)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return false
	}
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return false
			}
			rel, err = filepath.Rel(r.exec.Root, resolved)
			return err == nil && (rel == "." || filepath.IsLocal(rel))
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func javaClassName(file string, module javaModule, root string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(file))
	rel, err := filepath.Rel(module.TestSource, path)
	if err != nil || !filepath.IsLocal(rel) || !strings.HasSuffix(rel, ".java") {
		return "", fmt.Errorf("测试源码不在模块 testSourceDirectory")
	}
	return strings.ReplaceAll(strings.TrimSuffix(filepath.ToSlash(rel), ".java"), "/", "."), nil
}
