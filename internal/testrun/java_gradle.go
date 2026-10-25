package testrun

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func checkGradleSelection(dir, selector string, count int) error {
	file, err := os.Open(filepath.Join(dir, "selected.jsonl"))
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > outputLimit {
		return fmt.Errorf("Gradle 测试事件超出输出预算")
	}
	class, method, ok := strings.Cut(selector, "#")
	if !ok {
		return fmt.Errorf("Gradle 测试方法未核对")
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), outputLimit)
	seen := 0
	for scanner.Scan() {
		var event struct {
			Class, Name string
			Parents     []string
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return fmt.Errorf("Gradle 测试事件无效")
		}
		matched := false
		for _, name := range append(event.Parents, event.Name) {
			matched = matched || javaMethodMatches(class+"#"+name, class+"#"+method)
		}
		if event.Class != class || !matched {
			return fmt.Errorf("Gradle 实际测试超出方法 %s: %s %s", selector, event.Name, strings.Join(event.Parents, ","))
		}
		seen++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if seen != count {
		return fmt.Errorf("Gradle 事件与 JUnit 例数不一致")
	}
	return nil
}

const javaGradleInit = `import groovy.json.JsonOutput
import groovy.json.JsonSlurper
import org.gradle.api.tasks.testing.Test
gradle.projectsEvaluated {
  def output=new File(gradle.startParameter.projectProperties['taskbelay.output'])
  def modelTasks=[]
  def modelFiles=[]
  gradle.rootProject.allprojects.each { p ->
    if(p.plugins.hasPlugin('java')) {
      def sources=p.extensions.getByName('sourceSets')
      def modelFile=new File(output,'gradle-module-'+modelFiles.size()+'.json')
      modelFiles.add(modelFile)
      modelTasks.add(p.tasks.register('taskbelayModuleModel') {
        doLast {
          def t=p.tasks.named('test').get()
          def model=[ID:t.path, Source:sources.main.java.srcDirs.collect{it.canonicalPath}, TestSource:sources.test.java.srcDirs.collect{it.canonicalPath}, Classes:sources.main.output.classesDirs.files.collect{it.canonicalPath}, TestClasses:t.testClassesDirs.files.collect{it.canonicalPath}, Build:p.layout.buildDirectory.get().asFile.canonicalPath, JavaHome:t.javaLauncher.get().metadata.installationPath.asFile.canonicalPath, Classpath:t.classpath.files.collect{it.canonicalPath}, WorkingDir:t.workingDir.canonicalPath]
          modelFile.text=JsonOutput.toJson(model)
        }
      })
      p.tasks.withType(Test).configureEach { t ->
        if(t.name=='test') {
          def run=gradle.startParameter.projectProperties['taskbelay.run']
          if(run) {
            t.outputs.upToDateWhen { false }
            t.maxParallelForks=1
            t.reports.junitXml.required=true
            t.reports.junitXml.outputLocation=new File(run,'junit')
            t.reports.html.required=false
            t.binaryResultsDirectory.set(new File(run,'binary'))
            def jacoco=t.extensions.findByName('jacoco'); if(jacoco!=null) jacoco.enabled=false
            t.jvmArgs('-javaagent:'+gradle.startParameter.projectProperties['taskbelay.agent']+'=destfile='+new File(run,'coverage.exec').absolutePath+',append=false')
            t.doFirst { new File(run,'actual.json').text=JsonOutput.toJson([Classpath:t.classpath.files.collect{it.canonicalPath}, WorkingDir:t.workingDir.canonicalPath]) }
            t.afterTest { descriptor, result ->
              def parents=[]; def parent=descriptor.parent
              while(parent!=null) { parents.add(parent.name); parent=parent.parent }
              new File(run,'selected.jsonl').append(JsonOutput.toJson([Class:descriptor.className,Name:descriptor.name,Parents:parents])+'\n','UTF-8')
            }
          }
        }
      }
      p.tasks.named('test').get()
    }
  }
  gradle.rootProject.tasks.register('taskbelayModel') {
    dependsOn(modelTasks)
    doLast { new File(output,'gradle-model.json').text=JsonOutput.toJson(modelFiles.collect{new JsonSlurper().parse(it)}) }
  }
}`

type gradleModule struct {
	ID, Build, JavaHome, WorkingDir                     string
	Source, TestSource, Classes, TestClasses, Classpath []string
}

func (r *JavaRunner) gradleArgs(args ...string) []string {
	options := []string{"--init-script", r.gradleInit, "--project-cache-dir", filepath.Join(r.exec.Output, "gradle-project"), "-Porg.gradle.java.installations.auto-download=false", "-Ptaskbelay.output=" + r.exec.Output}
	return r.command.args(append(options, args...)...)
}

func (r *JavaRunner) prepareGradle(ctx context.Context) Result {
	home := filepath.Join(r.exec.Output, "gradle-home")
	if err := os.MkdirAll(home, 0700); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	if err := r.command.prepareWrapper(ctx, r.exec, r.caches, home); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	for _, cache := range r.caches {
		if _, err := os.Stat(filepath.Join(cache, "caches", "modules-2")); err != nil {
			continue
		}
		if err := copyJavaCache(ctx, filepath.Join(cache, "caches", "modules-2"), filepath.Join(home, "caches", "modules-2")); err != nil {
			return failure(EnvironmentFailure, err.Error())
		}
	}
	r.command.env = mergeEnvironment(r.command.env, []string{"GRADLE_USER_HOME=" + home})
	r.gradleInit = filepath.Join(r.exec.Output, "taskbelay.gradle")
	if err := os.WriteFile(r.gradleInit, []byte(javaGradleInit), 0600); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	run := r.exec.Execute(ctx, "prepare", r.exec.Root, r.gradleArgs("taskbelayModel"), r.command.env)
	if !run.Passed() {
		return run
	}
	var modules []gradleModule
	data, err := os.ReadFile(filepath.Join(r.exec.Output, "gradle-model.json"))
	if err != nil || len(data) > outputLimit || json.Unmarshal(data, &modules) != nil {
		return failure(EnvironmentFailure, "Gradle 模块、JDK 与源码位置未完整返回")
	}
	for _, model := range modules {
		if len(model.Source) != 1 || len(model.TestSource) != 1 || len(model.Classes) != 1 || len(model.TestClasses) != 1 {
			return failure(EnvironmentFailure, "Gradle 自定义 source set 尚不能唯一映射 Java 源码")
		}
		module := javaModule{ID: model.ID, Source: model.Source[0], TestSource: model.TestSource[0], Classes: model.Classes[0], TestClasses: model.TestClasses[0], Build: model.Build}
		for _, path := range []string{module.Source, module.TestSource, module.Classes, module.TestClasses, module.Build} {
			if !r.inside(path) {
				return failure(EnvironmentFailure, "Gradle 源码或输出越出固定副本: "+path)
			}
		}
		if err := r.javaClasspath(model.Classpath, model.WorkingDir); err != nil {
			return failure(EnvironmentFailure, err.Error())
		}
		if r.javaHome != "" && r.javaHome != model.JavaHome {
			return failure(EnvironmentFailure, "多个 Gradle 测试 JDK 尚不能共同核对")
		}
		r.javaHome = model.JavaHome
		r.modules = append(r.modules, module)
	}
	if len(r.modules) == 0 {
		return failure(SelectionFailure, "Gradle 没有可核对的 Java test 任务")
	}
	return Result{}
}

func (r *JavaRunner) javaClasspath(paths []string, dir string) error {
	if !sameDirectory(dir, r.exec.Root) && !r.inside(dir) {
		return fmt.Errorf("Java 测试工作目录位于固定副本之外")
	}
	for _, path := range paths {
		if path == "" || path == "." || sameDirectory(path, r.exec.Root) {
			continue
		}
		if r.inside(path) {
			continue
		}
		if _, inside := copyFile(r.exec.Workspace, path); inside {
			return fmt.Errorf("Java classpath 指向原工作区: %s", path)
		}
		if !strings.HasSuffix(path, ".jar") {
			return fmt.Errorf("Java 外部 classpath 不是已核对的依赖 jar: %s", path)
		}
	}
	return nil
}

func (r *JavaRunner) gradleCompile(ctx context.Context) Result {
	var tasks []string
	for _, module := range r.modules {
		tasks = append(tasks, strings.TrimSuffix(module.ID, "test")+"testClasses")
	}
	return r.exec.Execute(ctx, "compile", r.exec.Root, r.gradleArgs(tasks...), r.command.env)
}

func (r *JavaRunner) gradleRun(ctx context.Context, module javaModule, selector, dir, stage string) Result {
	return r.exec.Execute(ctx, stage, r.exec.Root, r.gradleArgs(module.ID, "--tests", strings.Replace(selector, "#", ".", 1), "-Ptaskbelay.run="+dir, "-Ptaskbelay.agent="+r.agent), r.command.env)
}

func copyJavaCache(ctx context.Context, source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), ".lock") || entry.Name() == "gc.properties" {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("依赖缓存不是普通文件: %s", path)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		info, err := entry.Info()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600|info.Mode().Perm()&0111)
		if os.IsExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = io.Copy(output, cancelReader{ctx, input})
		closeErr := output.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
