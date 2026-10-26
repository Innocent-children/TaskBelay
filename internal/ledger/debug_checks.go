package ledger

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type debugCheckReference struct {
	Path           string      `json:"check_ref"`
	SHA256         string      `json:"sha256"`
	Bytes          int         `json:"bytes"`
	ID             int64       `json:"id"`
	Depth          string      `json:"depth"`
	InputVersion   string      `json:"input_version"`
	Status         CheckStatus `json:"status"`
	ComputedStatus CheckStatus `json:"computed_status"`
	Level          string      `json:"level"`
}

type debugValue struct {
	root      *os.Root
	directory string
	task      string
	value     any
}

/**
 * MarshalJSON 只替换调试副本中的检查正文。按实际类型处理，避免把用户参数中
 * 恰好同名的字段当成检查；业务返回值和账本中的结构保持原样。
 */
func (v debugValue) MarshalJSON() ([]byte, error) {
	wrap := func(value any) debugValue {
		return debugValue{root: v.root, directory: v.directory, task: v.task, value: value}
	}
	optional := func(record *CheckRecord) any {
		if record == nil {
			return nil
		}
		return wrap(*record)
	}
	switch value := v.value.(type) {
	case CheckRecord:
		data, err := json.Marshal(struct {
			CheckRecord
			Static any `json:"static,omitempty"`
		}{value, optional(value.Static)})
		if err != nil {
			return nil, err
		}
		reference, err := v.saveCheck(data)
		if err != nil {
			return nil, err
		}
		reference.ID, reference.Depth, reference.InputVersion = value.ID, value.Depth, value.InputVersion
		reference.Status, reference.ComputedStatus, reference.Level = value.Status, value.ComputedStatus, value.Level
		return json.Marshal(reference)
	case *CheckRecord:
		return json.Marshal(optional(value))
	case []CheckRecord:
		var records []debugValue
		if value != nil {
			records = make([]debugValue, len(value))
			for i := range value {
				records[i] = wrap(value[i])
			}
		}
		return json.Marshal(records)
	case TaskView:
		return json.Marshal(struct {
			TaskView
			Checks debugValue `json:"checks"`
		}{value, wrap(value.Checks)})
	case FinishResult:
		return json.Marshal(struct {
			FinishResult
			Static  any `json:"static,omitempty"`
			Dynamic any `json:"dynamic,omitempty"`
		}{value, optional(value.Static), optional(value.Dynamic)})
	case Delivery:
		return json.Marshal(struct {
			Delivery
			Check debugValue `json:"check"`
		}{value, wrap(value.Check)})
	case map[string]any:
		if value == nil {
			return []byte("null"), nil
		}
		fields := make(map[string]debugValue, len(value))
		for name, field := range value {
			fields[name] = wrap(field)
		}
		return json.Marshal(fields)
	default:
		return json.Marshal(value)
	}
}

/**
 * saveCheck 先完成正文再发布引用。内容地址跨进程复用，日期和任务目录让一份
 * 日志可以连同正文独立保存；并发写入者即使同时发布，正文内容也相同。
 */
func (v debugValue) saveCheck(data []byte) (debugCheckReference, error) {
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	relative := filepath.Join(v.task+".checks", digest+".json")
	path := filepath.Join(v.directory, relative)
	reference := debugCheckReference{Path: filepath.ToSlash(relative), SHA256: digest, Bytes: len(data)}
	if info, err := v.root.Stat(path); err == nil {
		if !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
			return reference, fmt.Errorf("debug 检查正文不完整: %s", path)
		}
		return reference, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return reference, err
	}
	if err := v.root.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return reference, err
	}
	temporary := filepath.Join(filepath.Dir(path), ".tmp-"+rand.Text())
	file, err := v.root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return reference, err
	}
	defer v.root.Remove(temporary)
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return reference, err
	}
	if err := v.root.Rename(temporary, path); err != nil {
		// 部分平台拒绝替换另一进程刚发布的同名正文。
		if info, statErr := v.root.Stat(path); statErr == nil && info.Mode().IsRegular() && info.Size() == int64(len(data)) {
			return reference, nil
		}
		return reference, err
	}
	return reference, nil
}
