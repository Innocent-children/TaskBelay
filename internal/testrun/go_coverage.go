package testrun

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (g *GoRunner) ReadCoverage(ctx context.Context, run RunResult) Coverage {
	file, err := os.Open(run.Profile)
	if err != nil {
		return Coverage{Reason: "覆盖文件未生成: " + err.Error()}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || scanner.Text() != "mode: set" {
		return Coverage{Reason: "Go 覆盖模式或头部无效"}
	}
	covered := map[Line]bool{}
	blocks := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return Coverage{Reason: err.Error()}
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return Coverage{Reason: "Go 覆盖行格式无效"}
		}
		at := strings.LastIndexByte(fields[0], ':')
		if at < 0 {
			return Coverage{Reason: "Go 覆盖没有源码位置"}
		}
		source := filepath.ToSlash(fields[0][:at])
		path := g.coverageFiles[source]
		if path == "" {
			return Coverage{Reason: "覆盖源码不能定位到固定副本: " + source}
		}
		var first, firstColumn, last, lastColumn int
		if n, err := fmt.Sscanf(fields[0][at+1:], "%d.%d,%d.%d", &first, &firstColumn, &last, &lastColumn); err != nil || n != 4 || first < 1 || last < first || firstColumn < 1 || lastColumn < 1 {
			return Coverage{Reason: "Go 覆盖区间无效"}
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return Coverage{Reason: "Go 覆盖次数无效"}
		}
		statements, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Coverage{Reason: "Go 覆盖语句数无效"}
		}
		maxLine := strings.Count(string(g.request.Snapshot.Files[path].Data), "\n") + 1
		if last > maxLine {
			return Coverage{Reason: "覆盖区间超出固定源码"}
		}
		blocks++
		if count == 0 || statements == 0 {
			continue
		}
		if lastColumn == 1 && last > first {
			last--
		}
		for line := first; line <= last; line++ {
			covered[Line{File: path, Number: line}] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return Coverage{Reason: err.Error()}
	}
	if blocks == 0 {
		return Coverage{Reason: "覆盖文件没有可核对的副本代码块"}
	}
	result := Coverage{Valid: true, CopyExecuted: true}
	for line := range covered {
		result.Lines = append(result.Lines, line)
	}
	sortLines(result.Lines)
	return result
}

func sortLines(lines []Line) {
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].File == lines[j].File {
			return lines[i].Number < lines[j].Number
		}
		return lines[i].File < lines[j].File
	})
}
