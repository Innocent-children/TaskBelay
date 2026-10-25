package testrun

import (
	"context"
	"crypto/md5"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (c *javaCommand) prepareWrapper(ctx context.Context, e *Executor, caches []string, target string) error {
	program := c.prefix[len(c.prefix)-1]
	name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(program), ".cmd"), ".bat")
	if name != "mvnw" && name != "gradlew" {
		return nil
	}
	property := ".mvn/wrapper/maven-wrapper.properties"
	if c.tool == "gradle" {
		property = "gradle/wrapper/gradle-wrapper.properties"
	}
	data, err := os.ReadFile(filepath.Join(e.Root, property))
	if err != nil {
		return fmt.Errorf("wrapper 启动配置无法核对: %w", err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.ReplaceAll(strings.TrimSpace(value), `\:`, ":")
		}
	}
	address := values["distributionUrl"]
	parsed, err := url.Parse(address)
	if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("wrapper 发行地址尚不能核对已有缓存")
	}
	for key, value := range values {
		if strings.Contains(strings.ToLower(key), "jdk") && value != "" {
			return fmt.Errorf("wrapper 自带 JDK 选择或安装尚不能核对")
		}
	}
	for _, entry := range mergeEnvironment(e.Env, c.env) {
		if value, ok := strings.CutPrefix(entry, "MVNW_REPOURL="); ok && value != "" {
			return fmt.Errorf("Maven wrapper 的发行镜像覆盖尚不能核对")
		}
	}
	archive := filepath.Base(parsed.Path)
	if !strings.HasSuffix(archive, ".zip") {
		return fmt.Errorf("wrapper 发行包格式尚不能核对")
	}
	version := strings.TrimSuffix(archive, ".zip")
	checksum := md5.Sum([]byte(address))
	hash := new(big.Int).SetBytes(checksum[:]).Text(36)
	if c.tool == "gradle" {
		for _, key := range []string{"distributionBase", "zipStoreBase"} {
			if value := values[key]; value != "" && value != "GRADLE_USER_HOME" {
				return fmt.Errorf("Gradle wrapper 自定义存储位置尚不能核对")
			}
		}
		for _, key := range []string{"distributionPath", "zipStorePath"} {
			if value := values[key]; value != "" && value != "wrapper/dists" {
				return fmt.Errorf("Gradle wrapper 自定义存储位置尚不能核对")
			}
		}
		if _, err := os.Stat(filepath.Join(e.Root, "gradle/wrapper/gradle-wrapper.jar")); err != nil {
			return fmt.Errorf("Gradle wrapper jar 未包含在固定副本")
		}
	} else {
		script, err := os.ReadFile(program)
		if err != nil {
			return err
		}
		if runtime.GOOS == "windows" && strings.Contains(string(script), "MD5") {
			hash = fmt.Sprintf("%x", checksum)
		} else if strings.Contains(string(script), "hash_string()") {
			var value uint32
			for _, c := range []byte(address) {
				if c >= 128 {
					return fmt.Errorf("Maven wrapper 非 ASCII 发行地址尚不能核对")
				}
				value = value*31 + uint32(c)
			}
			hash = fmt.Sprintf("%x", value)
		} else {
			return fmt.Errorf("Maven wrapper 缓存格式未确认，不自动下载发行包")
		}
		version = strings.TrimSuffix(version, "-bin")
	}
	relative := filepath.Join("wrapper", "dists", version, hash)
	for _, cache := range caches {
		if c.tool == "maven" && filepath.Base(cache) == "repository" {
			cache = filepath.Dir(cache)
		}
		source := filepath.Join(cache, relative)
		binary := "mvn"
		if runtime.GOOS == "windows" {
			binary += ".cmd"
		}
		if c.tool == "gradle" {
			if _, err := os.Stat(filepath.Join(source, archive+".ok")); err != nil {
				continue
			}
			matches, _ := filepath.Glob(filepath.Join(source, "gradle-*", "bin", "gradle"))
			if len(matches) != 1 {
				continue
			}
		} else if _, err := os.Stat(filepath.Join(source, "bin", binary)); err != nil {
			continue
		}
		if err := copyJavaCache(ctx, source, filepath.Join(target, relative)); err != nil {
			return err
		}
		key := "MAVEN_USER_HOME"
		if c.tool == "gradle" {
			key = "GRADLE_USER_HOME"
		}
		c.env = mergeEnvironment(c.env, []string{key + "=" + target})
		return nil
	}
	return fmt.Errorf("项目 wrapper 对应的发行包尚未完整缓存在本机，不自动下载")
}
