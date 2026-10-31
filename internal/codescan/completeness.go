package codescan

import (
	"context"
	"errors"
)

func failureKind(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "incomplete"
	}
	return "error"
}

/**
 * 操作只传播实际记录的原因类型；语法失败会阻断依赖语法树的操作。
 * 无分类的缺口仍是不完整，不能根据提示文字推断为已支持或可复用。
 */
func finishOperations(file *File) {
	for name, operation := range file.Operations {
		if name == "files" {
			continue
		}
		kind := ""
		for _, missing := range file.Missing {
			if missing.Operation != name && missing.Operation != "analysis" && missing.Operation != "syntax" && !(missing.Operation == "declarations" && (name == "references" || name == "usage")) {
				continue
			}
			candidate := missing.Kind
			if candidate == "" {
				candidate = "incomplete"
			}
			if kind == "" || candidate == "error" || kind == "unsupported" {
				kind = candidate
			}
			if operation.Reason == "" {
				operation.Reason = missing.Reason
			}
		}
		if kind != "" {
			operation.Complete, operation.Kind = false, kind
		} else if !operation.Complete && operation.Kind == "" {
			operation.Kind = "incomplete"
		}
		file.Operations[name] = operation
	}
}

func (f *File) GapKind(operations ...string) string {
	kind := "unsupported"
	for _, name := range operations {
		op := f.Operations[name]
		if op.Complete {
			continue
		}
		if op.Kind == "error" {
			return "error"
		}
		if op.Kind != "unsupported" {
			kind = "incomplete"
		}
	}
	return kind
}
