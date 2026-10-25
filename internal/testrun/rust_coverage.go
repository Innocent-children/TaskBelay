package testrun

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
)

func readLCOV(ctx context.Context, root, original, path string, snapshot codescan.Snapshot) Coverage {
	file, err := os.Open(path)
	if err != nil {
		return Coverage{Reason: err.Error()}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > outputLimit {
		return Coverage{Reason: "LCOV 报告缺失或超出输出预算"}
	}
	result := Coverage{Valid: true}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), outputLimit)
	current := ""
	records, ended := 0, true
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return Coverage{Reason: err.Error()}
		}
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, "SF:"); ok {
			if !ended {
				return Coverage{Reason: "LCOV 文件记录未结束"}
			}
			ended = false
			records++
			current = ""
			rel, inside := copyFile(root, name)
			if !inside {
				if original != "" {
					if external, known := copyFile(original, name); known && snapshot.Files[external] != nil {
						return Coverage{Reason: "LCOV 来自原工作区: " + external}
					}
				}
				continue
			}
			if source := snapshot.Files[rel]; source != nil && source.Kind.Language == "rust" {
				current = rel
			}
		} else if data, ok := strings.CutPrefix(line, "DA:"); ok {
			if ended {
				return Coverage{Reason: "LCOV 行记录缺少文件身份"}
			}
			parts := strings.Split(data, ",")
			if len(parts) < 2 {
				return Coverage{Reason: "LCOV 行记录无法解析"}
			}
			number, e1 := strconv.Atoi(parts[0])
			count, e2 := strconv.ParseUint(parts[1], 10, 64)
			if e1 != nil || e2 != nil || number <= 0 {
				return Coverage{Reason: "LCOV 行号或执行次数无效"}
			}
			if current == "" {
				continue
			}
			if number > lineCount(snapshot.Files[current].Data) {
				return Coverage{Reason: fmt.Sprintf("LCOV 行超出固定源码: %s:%d", current, number)}
			}
			if count > 0 {
				result.CopyExecuted = true
				result.Lines = append(result.Lines, Line{File: current, Number: number})
			}
		} else if line == "end_of_record" {
			ended = true
			current = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return Coverage{Reason: err.Error()}
	}
	if !ended || records == 0 {
		return Coverage{Reason: "LCOV 没有完整的文件记录"}
	}
	result.Lines = normalizedLines(result.Lines)
	if !result.CopyExecuted {
		result.Reason = "覆盖数据没有副本源码执行证据"
	}
	return result
}
