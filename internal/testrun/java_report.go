package testrun

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
)

func cachedJaCoCoAgent(caches []string, output string) (string, string, error) {
	if path, version := cachedJavaArtifact(caches, "org.jacoco", "org.jacoco.agent", "*", "-runtime"); path != "" {
		return path, version, nil
	}
	path, version := cachedJavaArtifact(caches, "org.jacoco", "org.jacoco.agent", "*", "")
	if path == "" {
		return "", "", fmt.Errorf("已有 Maven 仓库或 Gradle 缓存缺少 JaCoCo agent")
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", "", err
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name != "jacocoagent.jar" {
			continue
		}
		if entry.UncompressedSize64 > outputLimit {
			return "", "", fmt.Errorf("JaCoCo agent 超出读取预算")
		}
		input, err := entry.Open()
		if err != nil {
			return "", "", err
		}
		defer input.Close()
		data, err := io.ReadAll(io.LimitReader(input, outputLimit+1))
		if err != nil || len(data) > outputLimit {
			return "", "", fmt.Errorf("JaCoCo agent 读取未完成")
		}
		target := filepath.Join(output, "jacocoagent.jar")
		if err := os.WriteFile(target, data, 0600); err != nil {
			return "", "", err
		}
		return target, version, nil
	}
	return "", "", fmt.Errorf("缓存 JaCoCo 包没有 agent")
}

type junitCase struct {
	Name    string `xml:"name,attr"`
	Class   string `xml:"classname,attr"`
	Failure *struct {
		Type    string `xml:"type,attr"`
		Message string `xml:",chardata"`
	} `xml:"failure"`
	Error *struct {
		Type string `xml:"type,attr"`
	} `xml:"error"`
	Skipped *struct{} `xml:"skipped"`
}

type junitSuite struct {
	Name       string      `xml:"name,attr"`
	Cases      []junitCase `xml:"testcase"`
	Properties []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"properties>property"`
}

func readXML(path string, out any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > outputLimit {
		return fmt.Errorf("XML 报告超出输出预算")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return xml.Unmarshal(data, out)
}

func readJUnit(dir string) ([]junitSuite, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "TEST-*.xml"))
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("JUnit 没有本次执行报告")
	}
	var result []junitSuite
	for _, path := range paths {
		var suite junitSuite
		if err := readXML(path, &suite); err != nil {
			return nil, err
		}
		result = append(result, suite)
	}
	return result, nil
}

func javaAssertionFailure(kind string) bool {
	return kind == "java.lang.AssertionError" || kind == "org.opentest4j.AssertionFailedError" || kind == "org.opentest4j.MultipleFailuresError" || kind == "junit.framework.AssertionFailedError" || kind == "org.junit.ComparisonFailure"
}

func junitName(test junitCase) string { return test.Class + "#" + test.Name }

func readJaCoCo(ctx context.Context, path, sourceRoot, root string, snapshot codescan.Snapshot) Coverage {
	var report struct {
		XMLName  xml.Name `xml:"report"`
		Packages []struct {
			Name    string `xml:"name,attr"`
			Sources []struct {
				Name  string `xml:"name,attr"`
				Lines []struct {
					Number  int `xml:"nr,attr"`
					Covered int `xml:"ci,attr"`
					Missed  int `xml:"mi,attr"`
				} `xml:"line"`
			} `xml:"sourcefile"`
		} `xml:"package"`
	}
	if err := readXML(path, &report); err != nil {
		return Coverage{Reason: err.Error()}
	}
	result := Coverage{Valid: true}
	for _, pkg := range report.Packages {
		for _, source := range pkg.Sources {
			if err := ctx.Err(); err != nil {
				return Coverage{Reason: err.Error()}
			}
			if !filepath.IsLocal(filepath.FromSlash(pkg.Name + "/" + source.Name)) {
				return Coverage{Reason: "JaCoCo 源码路径越出模块"}
			}
			file, inside := copyFile(root, filepath.Join(sourceRoot, filepath.FromSlash(pkg.Name), source.Name))
			fixed := snapshot.Files[file]
			if !inside || fixed == nil || fixed.Kind.Language != "java" {
				return Coverage{Reason: "JaCoCo 源码不在固定副本: " + source.Name}
			}
			for _, line := range source.Lines {
				if line.Number < 1 || line.Number > lineCount(fixed.Data) || line.Covered < 0 || line.Missed < 0 {
					return Coverage{Reason: "JaCoCo 行记录无效"}
				}
				if line.Covered > 0 {
					result.CopyExecuted = true
					result.Lines = append(result.Lines, Line{File: file, Number: line.Number})
				}
			}
		}
	}
	result.Lines = normalizedLines(result.Lines)
	if !result.CopyExecuted {
		result.Reason = "JaCoCo 没有副本生产源码的执行证据"
	}
	return result
}

func versionLess(a, b string) int {
	x, y := strings.FieldsFunc(a, func(c rune) bool { return c < '0' || c > '9' }), strings.FieldsFunc(b, func(c rune) bool { return c < '0' || c > '9' })
	for i := 0; i < min(len(x), len(y)); i++ {
		n, _ := strconv.Atoi(x[i])
		m, _ := strconv.Atoi(y[i])
		if n != m {
			return n - m
		}
	}
	return len(x) - len(y)
}

func cachedJavaArtifact(caches []string, group, artifact, version, suffix string) (string, string) {
	var matches []string
	for _, cache := range caches {
		pattern := filepath.Join(cache, filepath.FromSlash(strings.ReplaceAll(group, ".", "/")), artifact, version, artifact+"-*"+suffix+".jar")
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
		pattern = filepath.Join(cache, "caches", "modules-2", "files-2.1", group, artifact, version, "*", artifact+"-*"+suffix+".jar")
		found, _ = filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	var usable []string
	for _, path := range matches {
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), artifact+"-"), suffix+".jar")
		if base == "" || strings.ContainsAny(base, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			continue
		}
		usable = append(usable, path)
	}
	slices.SortFunc(usable, func(a, b string) int { return versionLess(filepath.Base(b), filepath.Base(a)) })
	if len(usable) == 0 {
		return "", ""
	}
	path := usable[0]
	return path, strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), artifact+"-"), suffix+".jar")
}

// 只分析固定副本的 class 文件；同名类的执行数据与字节码不匹配时拒绝覆盖。
const javaCoverageReader = `import java.io.*;
import org.jacoco.core.analysis.*;
import org.jacoco.core.tools.ExecFileLoader;
import org.jacoco.report.*;
import org.jacoco.report.xml.XMLFormatter;
public final class TaskBelayCoverage {
  public static void main(String[] args) throws Exception {
    ExecFileLoader data=new ExecFileLoader(); data.load(new File(args[0]));
    CoverageBuilder coverage=new CoverageBuilder();
    new Analyzer(data.getExecutionDataStore(),coverage).analyzeAll(new File(args[1]));
    if(!coverage.getNoMatchClasses().isEmpty()) throw new IOException("coverage class identity mismatch");
    try(OutputStream output=new FileOutputStream(args[3])) {
      IReportVisitor report=new XMLFormatter().createVisitor(output);
      report.visitInfo(data.getSessionInfoStore().getInfos(),data.getExecutionDataStore().getContents());
      report.visitBundle(coverage.getBundle("fixed-copy"),new DirectorySourceFileLocator(new File(args[2]),"UTF-8",4));
      report.visitEnd();
    }
  }
}`
