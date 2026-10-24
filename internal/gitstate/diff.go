package gitstate

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// FileChange 是两个工作区状态之间一个文件的改动。
type FileChange struct {
	Path   string // 相对工作区根目录，用 / 分隔
	Status Status
	Hunks  []Hunk // 二进制文件和只改了权限的文件没有 Hunk
}

// Status 是文件改动的种类。
type Status byte

const (
	Added    Status = 'A'
	Modified Status = 'M'
	Deleted  Status = 'D'
)

// Hunk 是一段连续改动的行。行号从 1 开始；Count 为 0 表示这一侧没有行，
// 这时 Start 是改动发生位置的前一行。
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
}

// Diff 算出工作区状态 from 到 to 之间的改动，精确到行。改名按删除加新增算。
func Diff(root, from, to string) ([]FileChange, error) {
	out, err := git(root, nil,
		"-c", "core.quotepath=false",
		"diff", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "--unified=0",
		from, to)
	if err != nil {
		return nil, err
	}
	return parseDiff(out)
}

// parseDiff 解析 git diff 的输出。改动的内容行都以 +、-、空格或 \ 开头，
// 所以以 "diff --git " 和 "@@ " 开头的行一定是文件头和 Hunk 头。
func parseDiff(out string) ([]FileChange, error) {
	var changes []FileChange
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			path, err := headerPath(strings.TrimPrefix(line, "diff --git "))
			if err != nil {
				return nil, err
			}
			changes = append(changes, FileChange{Path: path, Status: Modified})
		case len(changes) == 0:
			continue
		case strings.HasPrefix(line, "new file mode "):
			changes[len(changes)-1].Status = Added
		case strings.HasPrefix(line, "deleted file mode "):
			changes[len(changes)-1].Status = Deleted
		case strings.HasPrefix(line, "@@ "):
			h, err := parseHunk(line)
			if err != nil {
				return nil, err
			}
			last := &changes[len(changes)-1]
			last.Hunks = append(last.Hunks, h)
		}
	}
	return changes, sc.Err()
}

// headerPath 从 "a/<路径> b/<路径>" 里取出路径。不改名时两边的路径相同；
// 路径里有特殊字符时，git 会给两边各加上 C 风格的双引号。
func headerPath(s string) (string, error) {
	if strings.HasPrefix(s, `"`) {
		end := closingQuote(s)
		if end < 0 {
			return "", fmt.Errorf("看不懂的 diff 文件头：%s", s)
		}
		unquoted, err := strconv.Unquote(s[:end+1])
		if err != nil {
			return "", fmt.Errorf("看不懂的 diff 文件头：%s", s)
		}
		return strings.TrimPrefix(unquoted, "a/"), nil
	}
	// s 是 "a/" + 路径 + " b/" + 路径，两段路径一样长。
	n := (len(s) - len("a/ b/")) / 2
	if n <= 0 || !strings.HasPrefix(s, "a/") {
		return "", fmt.Errorf("看不懂的 diff 文件头：%s", s)
	}
	return s[len("a/") : len("a/")+n], nil
}

// closingQuote 返回 s 开头那个双引号字符串的结束引号位置，找不到时返回 -1。
func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// parseHunk 解析 "@@ -旧起始[,旧行数] +新起始[,新行数] @@" 形式的 Hunk 头。
func parseHunk(line string) (Hunk, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return Hunk{}, fmt.Errorf("看不懂的 Hunk 头：%s", line)
	}
	oldStart, oldCount, err1 := parseRange(fields[1][1:])
	newStart, newCount, err2 := parseRange(fields[2][1:])
	if err1 != nil || err2 != nil {
		return Hunk{}, fmt.Errorf("看不懂的 Hunk 头：%s", line)
	}
	return Hunk{OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount}, nil
}

// parseRange 解析 "起始,行数" 或 "起始"，后者表示 1 行。
func parseRange(s string) (start, count int, err error) {
	startText, countText, hasCount := strings.Cut(s, ",")
	if start, err = strconv.Atoi(startText); err != nil {
		return 0, 0, err
	}
	if !hasCount {
		return start, 1, nil
	}
	count, err = strconv.Atoi(countText)
	return start, count, err
}
