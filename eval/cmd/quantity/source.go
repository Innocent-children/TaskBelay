package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/eval/internal/rustinputs"
	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

type fileQuantity struct {
	sourceLines   []bool
	sourceUnknown string
	testUnknown   []string
}

func fileQuantities(snapshot codescan.Snapshot) map[string]fileQuantity {
	result := map[string]fileQuantity{}
	testReferences := map[string]bool{}
	for name, file := range snapshot.Files {
		facts := fileQuantity{sourceLines: make([]bool, lineCount(file.Data))}
		if !regular(file.Mode) {
			facts.sourceUnknown = "非普通源码文件，未跟随符号链接或展开子模块"
			facts.testUnknown = []string{facts.sourceUnknown}
			result[name] = facts
			continue
		}
		if file.Kind.TestData {
			result[name] = facts
			continue
		}
		if !file.Kind.Test {
			for i := range facts.sourceLines {
				facts.sourceLines[i] = true
			}
		}
		if file.Kind.Language == "rust" {
			parsed := rustinputs.Describe(rustinputs.Input{Path: name, Data: file.Data})
			if parsed.Error != "" {
				if !file.Kind.Test {
					facts.sourceUnknown = "Rust 测试范围未核对：" + parsed.Error
				}
				facts.testUnknown = append(facts.testUnknown, "Rust 测试输入未核对："+parsed.Error)
			} else {
				mask := make([]bool, len(file.Data))
				for _, item := range parsed.Items {
					if item.Test {
						for index := item.Start; index < item.End; index++ {
							mask[index] = true
						}
					}
					for _, control := range item.Control {
						if !strings.HasPrefix(control, "#[cfg") && !strings.HasPrefix(control, "#![cfg") {
							facts.testUnknown = append(facts.testUnknown, "Rust 宏可能生成测试，源码枚举不能确定新增用例数")
							break
						}
					}
				}
				if !file.Kind.Test {
					start := 0
					for i, line := range bytes.SplitAfter(file.Data, []byte{'\n'}) {
						if len(line) == 0 {
							continue
						}
						tested, production := false, false
						for offset, value := range line {
							if mask[start+offset] {
								tested = true
							} else if value != ' ' && value != '\t' && value != '\n' && value != '\r' {
								production = true
							}
						}
						facts.sourceLines[i] = !tested || production
						start += len(line)
					}
				}
				for _, reference := range parsed.References {
					if reference.Test && !reference.Doc {
						for _, choice := range reference.Choices {
							testReferences[choice] = true
						}
					}
				}
			}
		}
		result[name] = facts
	}
	for name := range testReferences {
		if file := snapshot.Files[name]; file != nil && !file.Kind.Test && file.Kind.Language == "rust" {
			facts := result[name]
			for _, production := range facts.sourceLines {
				if production {
					facts.sourceUnknown = "外部 Rust 测试模块或包含文件的生产共享范围尚未核对"
					break
				}
			}
			result[name] = facts
		}
	}
	return result
}

/**
 * 新增行只计生产范围。未改文字从测试范围转入生产范围时也计入，
 * 避免移除 cfg(test) 后沿用旧的测试分类而把新增实现算成零。
 */
func addedSourceLines(before, after []bool, fragments []gitstate.Fragment) (int, error) {
	count, oldPosition, newPosition := 0, 0, 0
	unchanged := func(oldEnd, newEnd int) error {
		if oldEnd < oldPosition || newEnd < newPosition || oldEnd > len(before) || newEnd > len(after) || oldEnd-oldPosition != newEnd-newPosition {
			return fmt.Errorf("Git 行片段与固定源码长度不一致")
		}
		for newPosition < newEnd {
			if after[newPosition] && !before[oldPosition] {
				count++
			}
			newPosition++
			oldPosition++
		}
		return nil
	}
	for _, fragment := range fragments {
		oldStart, newStart := fragment.BeforeStart, fragment.AfterStart
		if fragment.BeforeLines > 0 {
			oldStart--
		}
		if fragment.AfterLines > 0 {
			newStart--
		}
		if err := unchanged(oldStart, newStart); err != nil {
			return 0, err
		}
		if oldStart+fragment.BeforeLines > len(before) || newStart+fragment.AfterLines > len(after) {
			return 0, fmt.Errorf("Git 新增片段超出固定源码范围")
		}
		for _, production := range after[newStart : newStart+fragment.AfterLines] {
			if production {
				count++
			}
		}
		oldPosition, newPosition = oldStart+fragment.BeforeLines, newStart+fragment.AfterLines
	}
	err := unchanged(len(before), len(after))
	return count, err
}
