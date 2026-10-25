package gitstate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Change struct {
	Status       string
	BeforePath   string
	Path         string
	Before       File
	After        File
	UniqueRename bool
}

func (w Workspace) Diff(ctx context.Context, origin, current string) ([]Change, error) {
	if !validObjectID(origin) || !validObjectID(current) {
		return nil, fmt.Errorf("累计 diff 需要有效的起点和当前 tree")
	}
	out, err := w.command(ctx, nil, nil, "diff-tree", "-r", "--no-commit-id", "--raw", "--no-abbrev", "-z", "-M", origin, current)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(out), "\x00")
	var changes []Change
	for i := 0; i < len(parts)-1; {
		fields := strings.Fields(strings.TrimPrefix(parts[i], ":"))
		if len(fields) != 5 || i+1 >= len(parts) {
			return nil, fmt.Errorf("无法解析 diff 条目")
		}
		change := Change{Status: fields[4], BeforePath: parts[i+1], Path: parts[i+1], Before: File{fields[2], fields[0]}, After: File{fields[3], fields[1]}}
		i += 2
		if strings.HasPrefix(change.Status, "R") {
			if i >= len(parts) {
				return nil, fmt.Errorf("改名缺少目标路径")
			}
			change.Path = parts[i]
			i++
		}
		if change.Before.Mode == "000000" {
			change.Before = File{}
		}
		if change.After.Mode == "000000" {
			change.After = File{}
		}
		changes = append(changes, change)
	}
	// 相似度不足以确定身份；只有唯一对应的相同内容才能自动迁移逻辑位置。
	for i := range changes {
		c := &changes[i]
		if c.Status != "R100" {
			continue
		}
		beforeCount, afterCount := 0, 0
		for _, other := range changes {
			if other.Before == c.Before {
				beforeCount++
			}
			if other.After == c.After {
				afterCount++
			}
		}
		c.UniqueRename = beforeCount == 1 && afterCount == 1
	}
	return changes, nil
}

type Fragment struct {
	BeforeStart     int
	BeforeLines     int
	AfterStart      int
	AfterLines      int
	BeforeText      string `json:"-"`
	AfterText       string `json:"-"`
	BeforeContentID string
	AfterContentID  string
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func (w Workspace) Fragments(ctx context.Context, before, after File) ([]Fragment, error) {
	if before == after {
		return nil, nil
	}
	if before.Mode == "160000" || after.Mode == "160000" {
		return nil, nil
	}
	var err error
	if before.Blob == "" {
		before.Blob, err = w.WriteBlob(ctx, nil)
		if err != nil {
			return nil, err
		}
	}
	if after.Blob == "" {
		after.Blob, err = w.WriteBlob(ctx, nil)
		if err != nil {
			return nil, err
		}
	}
	out, err := w.command(ctx, nil, nil, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--text", "--unified=0", "--inter-hunk-context=0", before.Blob, after.Blob)
	if err != nil {
		return nil, err
	}
	var fragments []Fragment
	var last byte
	for _, line := range strings.SplitAfter(string(out), "\n") {
		if match := hunkHeader.FindStringSubmatch(line); match != nil {
			f := Fragment{}
			f.BeforeStart, _ = strconv.Atoi(match[1])
			f.AfterStart, _ = strconv.Atoi(match[3])
			f.BeforeLines, f.AfterLines = 1, 1
			if match[2] != "" {
				f.BeforeLines, _ = strconv.Atoi(match[2])
			}
			if match[4] != "" {
				f.AfterLines, _ = strconv.Atoi(match[4])
			}
			fragments = append(fragments, f)
			last = 0
			continue
		}
		if len(fragments) == 0 || line == "" {
			continue
		}
		f := &fragments[len(fragments)-1]
		switch line[0] {
		case '-':
			f.BeforeText += line[1:]
			last = '-'
		case '+':
			f.AfterText += line[1:]
			last = '+'
		case '\\':
			if last == '-' {
				f.BeforeText = strings.TrimSuffix(f.BeforeText, "\n")
			}
			if last == '+' {
				f.AfterText = strings.TrimSuffix(f.AfterText, "\n")
			}
		}
	}
	var separated []Fragment
	for _, fragment := range fragments {
		// 相邻的独立修改可能被 Git 合并为一个 hunk；等长行段可按原位置分别核对。
		if fragment.BeforeLines > 1 && fragment.BeforeLines == fragment.AfterLines {
			beforeLines := strings.SplitAfter(fragment.BeforeText, "\n")
			afterLines := strings.SplitAfter(fragment.AfterText, "\n")
			if len(beforeLines) >= fragment.BeforeLines && len(afterLines) >= fragment.AfterLines {
				for i := 0; i < fragment.BeforeLines; i++ {
					separated = append(separated, Fragment{BeforeStart: fragment.BeforeStart + i, BeforeLines: 1, AfterStart: fragment.AfterStart + i, AfterLines: 1, BeforeText: beforeLines[i], AfterText: afterLines[i]})
				}
				continue
			}
		}
		separated = append(separated, fragment)
	}
	fragments = separated
	for i := range fragments {
		fragments[i].BeforeContentID = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(fragments[i].BeforeText)))
		fragments[i].AfterContentID = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(fragments[i].AfterText)))
	}
	return fragments, nil
}
