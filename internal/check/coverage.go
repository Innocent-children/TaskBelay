package check

import (
	"sort"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

/**
 * 分母来自实际文件变化，不能只枚举解析成功的声明。
 * 文件级能力与代码级能力分开列出，避免空声明集合被解释成全部检查通过。
 */
func (s *staticAnalysis) coverage() {
	paths := map[string]bool{}
	for path := range s.before.Files {
		paths[path] = true
	}
	for path := range s.after.Files {
		paths[path] = true
	}
	for path := range paths {
		if !s.fileChanged(path) {
			continue
		}
		file := s.after.Files[path]
		if file == nil {
			file = s.before.Files[path]
		}
		coverage := ledger.FileCoverage{Path: path, Level: "file"}
		if excluded(file) {
			coverage.Level = "excluded"
		} else {
			if file.Kind.Source && file.CodeComplete {
				coverage.Level = "code"
			}
			for name, operation := range file.Operations {
				if operation.Complete {
					coverage.Operations = append(coverage.Operations, name)
				}
			}
			sort.Strings(coverage.Operations)
		}
		s.result.Report.Coverage = append(s.result.Report.Coverage, coverage)
	}
	sort.Slice(s.result.Report.Coverage, func(i, j int) bool {
		return s.result.Report.Coverage[i].Path < s.result.Report.Coverage[j].Path
	})
}
