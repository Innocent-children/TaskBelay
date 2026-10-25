package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

const marker = "TaskBelay user installation v1\n"
const releaseURL = "https://github.com/Innocent-children/TaskBelay/releases/latest/download/"

func ManagedPath(env platform.Env, executable string) (string, error) {
	target := filepath.Join(platform.UserBinDir(env), platform.ExecutableName())
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("程序不在安装脚本管理的位置 %s；请使用原安装方式维护", target)
	}
	current, err := os.Stat(executable)
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, current) {
		return "", fmt.Errorf("当前运行的程序不是 %s；请运行该路径下的程序", target)
	}
	path := filepath.Join(filepath.Dir(target), ".taskbelay-install")
	info, err = os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("缺少有效安装标记；请使用原安装方式维护程序")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if string(data) != marker {
		return "", fmt.Errorf("安装标记不属于 TaskBelay，未修改程序")
	}
	return target, nil
}

func lock(target string) (func(), error) {
	path := filepath.Join(filepath.Dir(target), ".taskbelay-install.lock")
	if err := os.Mkdir(path, 0o700); err != nil {
		return nil, fmt.Errorf("无法取得安装锁 %s（可能有其他安装操作正在进行）: %w", path, err)
	}
	return func() { _ = os.Remove(path) }, nil
}

func Update(ctx context.Context, env platform.Env, executable string) (string, error) {
	return update(ctx, env, executable, releaseURL, &http.Client{Timeout: 5 * time.Minute})
}

func update(ctx context.Context, env platform.Env, executable, baseURL string, client *http.Client) (string, error) {
	target, err := ManagedPath(env, executable)
	if err != nil {
		return "", err
	}
	unlock, err := lock(target)
	if err != nil {
		return "", err
	}
	defer unlock()
	asset := "taskbelay-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	var sums strings.Builder
	if err := download(ctx, client, baseURL+"SHA256SUMS", &sums, 1<<20); err != nil {
		return "", err
	}
	want, err := checksum(sums.String(), asset)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".taskbelay-download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	hash := sha256.New()
	err = download(ctx, client, baseURL+asset, io.MultiWriter(f, hash), 512<<20)
	err = errors.Join(err, f.Close())
	if err != nil {
		return "", err
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return "", fmt.Errorf("下载包 SHA-256 校验失败，已保留旧版")
	}
	if err := os.Chmod(f.Name(), 0o755); err != nil {
		return "", err
	}
	leftover, err := platform.ReplaceExecutable(f.Name(), target)
	if err != nil {
		return "", fmt.Errorf("替换程序失败: %w", err)
	}
	return "已更新程序；请重新运行 taskbelay，并重启使用旧版的 Agent session。\n" + retiredMessage(leftover), nil
}

func download(ctx context.Context, client *http.Client, url string, output io.Writer, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败，原程序保留: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 返回 HTTP %d，原程序保留；尚无发布包时请从源码安装", url, resp.StatusCode)
	}
	n, err := io.Copy(output, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("下载文件超出大小限制，原程序保留")
	}
	return nil
}

func checksum(manifest, asset string) (string, error) {
	var result string
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size || result != "" {
			return "", fmt.Errorf("%s 的 SHA-256 清单无效", asset)
		}
		result = strings.ToLower(fields[0])
	}
	if result == "" {
		return "", fmt.Errorf("SHA-256 清单没有 %s，未修改程序", asset)
	}
	return result, nil
}

func Uninstall(env platform.Env, executable string) (string, error) {
	target, err := ManagedPath(env, executable)
	if err != nil {
		return "", err
	}
	unlock, err := lock(target)
	if err != nil {
		return "", err
	}
	defer unlock()
	leftover, err := platform.RemoveExecutable(target)
	if err != nil {
		return "", err
	}
	message := "已移除 TaskBelay 程序，用户配置、账本和 PATH 目录条目保留。\n" + retiredMessage(leftover)
	return message, os.Remove(filepath.Join(filepath.Dir(target), ".taskbelay-install"))
}

func retiredMessage(path string) string {
	if path == "" {
		return ""
	}
	return "旧程序仍被占用；关闭使用旧版的 Agent 后可删除此文件：" + path + "\n"
}
