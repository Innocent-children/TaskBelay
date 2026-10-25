package plan

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func Prepare(view ledger.TaskView, input ledger.PlanInput, dataDir string) (ledger.PlanRecord, error) {
	if view.Task == nil {
		return ledger.PlanRecord{}, fmt.Errorf("保存方案前先建立任务")
	}
	if strings.TrimSpace(input.Content) == "" {
		return ledger.PlanRecord{}, fmt.Errorf("方案正文不能为空，请说明原因、成功标准、做法和步骤")
	}
	record := ledger.PlanRecord{TaskID: view.Task.ID, Body: input.Content, Version: 1}
	var previous *ledger.PlanRecord
	if len(view.Plans.Versions) > 0 {
		copy := view.Plans.Versions[len(view.Plans.Versions)-1]
		previous = &copy
		record.Version = copy.Version + 1
	}
	record.ID = fmt.Sprintf("p-%s-v%d", record.TaskID, record.Version)
	var err error
	record.Path, err = filepath.Abs(filepath.Join(dataDir, "plans", record.TaskID, fmt.Sprintf("v%d.md", record.Version)))
	if err != nil {
		return record, err
	}
	fileIndex := map[string]int{}
	entryIndex := map[string]int{}
	depIndex := map[string]int{}
	if effective := view.Plans.Effective; effective != nil {
		record.Files = append(record.Files, effective.Files...)
		record.Entries = append(record.Entries, effective.Entries...)
		record.Dependencies = append(record.Dependencies, effective.Dependencies...)
	}
	for i, file := range record.Files {
		fileIndex[file.Path] = i
	}
	for i, entry := range record.Entries {
		entryIndex[entry.At] = i
	}
	for i, dep := range record.Dependencies {
		depIndex[dep.Module] = i
	}
	for _, file := range input.Files {
		file.Path, err = ScopePath(file.Path)
		if err != nil {
			return record, err
		}
		if strings.TrimSpace(file.Note) == "" || strings.TrimSpace(file.Change) == "" {
			return record, fmt.Errorf("范围 %s 缺少改动目的或类型", file.Path)
		}
		if i, ok := fileIndex[file.Path]; ok {
			record.Files[i] = file
		} else {
			fileIndex[file.Path] = len(record.Files)
			record.Files = append(record.Files, file)
		}
	}
	for _, entry := range input.Entries {
		valid := false
		for _, prefix := range []string{"flag:", "cmd:", "env:", "route:", "config:"} {
			valid = valid || strings.HasPrefix(entry.At, prefix) && len(entry.At) > len(prefix)
		}
		if !valid || strings.TrimSpace(entry.Note) == "" {
			return record, fmt.Errorf("对外入口需要文档规定的位置及用途: %s", entry.At)
		}
		if i, ok := entryIndex[entry.At]; ok {
			record.Entries[i] = entry
		} else {
			entryIndex[entry.At] = len(record.Entries)
			record.Entries = append(record.Entries, entry)
		}
	}
	for _, dep := range input.Dependencies {
		if strings.TrimSpace(dep.Module) == "" || strings.TrimSpace(dep.Reason) == "" {
			return record, fmt.Errorf("直接依赖需要模块名及引入理由")
		}
		if i, ok := depIndex[dep.Module]; ok {
			record.Dependencies[i] = dep
		} else {
			depIndex[dep.Module] = len(record.Dependencies)
			record.Dependencies = append(record.Dependencies, dep)
		}
	}
	record.Changes = Changes(previous, record)
	record.Content = Render(record)
	return record, nil
}

func ScopePath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	dir := strings.HasSuffix(value, "/")
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, ":") || strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("方案范围须为工作区内的相对文件或目录")
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("方案范围不能越出工作区")
	}
	if clean == "." && !dir {
		return "", fmt.Errorf("工作区目录范围须明确写为 ./")
	}
	if dir {
		clean += "/"
	}
	return clean, nil
}

func Contains(scope, file string) bool {
	if scope == "./" {
		return true
	}
	if strings.HasSuffix(scope, "/") {
		return strings.HasPrefix(file, scope)
	}
	return scope == file
}

func Changes(previous *ledger.PlanRecord, current ledger.PlanRecord) []string {
	if previous == nil {
		return []string{"首次提交完整方案。"}
	}
	var changes []string
	if previous.Body != current.Body {
		changes = append(changes, "调整方案正文，请结合完整内容核对原因、成功标准及步骤。")
	}
	old, new := scopeDescriptions(*previous), scopeDescriptions(current)
	for _, key := range scopeOrder(current) {
		if old[key] != new[key] {
			verb := "调整"
			if old[key] == "" {
				verb = "新增"
			}
			changes = append(changes, verb+key+"："+new[key])
		}
	}
	for _, key := range scopeOrder(*previous) {
		if _, ok := new[key]; !ok {
			changes = append(changes, "移除"+key+"。")
		}
	}
	if len(changes) == 0 {
		changes = append(changes, "范围和正文与上一版相同。")
	}
	return changes
}

func scopeDescriptions(record ledger.PlanRecord) map[string]string {
	result := map[string]string{}
	for _, file := range record.Files {
		result["范围 "+file.Path] = file.Change + "；" + file.Note
	}
	for _, entry := range record.Entries {
		result["入口 "+entry.At] = entry.Note
	}
	for _, dep := range record.Dependencies {
		result["依赖 "+dep.Module] = dep.Reason
	}
	return result
}
func scopeOrder(record ledger.PlanRecord) []string {
	var result []string
	for _, file := range record.Files {
		result = append(result, "范围 "+file.Path)
	}
	for _, entry := range record.Entries {
		result = append(result, "入口 "+entry.At)
	}
	for _, dep := range record.Dependencies {
		result = append(result, "依赖 "+dep.Module)
	}
	return result
}

func Render(record ledger.PlanRecord) string {
	var text strings.Builder
	fmt.Fprintf(&text, "# 方案 v%d\n\n%s\n\n## 相对上一版的变更\n\n", record.Version, strings.TrimSpace(record.Body))
	for _, change := range record.Changes {
		fmt.Fprintln(&text, "- "+change)
	}
	fmt.Fprintln(&text, "\n## 涉及的改动\n\n以下是本版完整的累计范围。")
	for _, file := range record.Files {
		fmt.Fprintf(&text, "\n- %s（%s）：%s\n", file.Path, file.Change, file.Note)
	}
	if len(record.Files) == 0 {
		fmt.Fprintln(&text, "\n- 涉及范围：无。")
	}
	for _, entry := range record.Entries {
		fmt.Fprintf(&text, "\n- 新增入口 %s：%s\n", entry.At, entry.Note)
	}
	if len(record.Entries) == 0 {
		fmt.Fprintln(&text, "\n- 新增入口：无。")
	}
	for _, dep := range record.Dependencies {
		fmt.Fprintf(&text, "\n- 新增依赖 %s：%s\n", dep.Module, dep.Reason)
	}
	if len(record.Dependencies) == 0 {
		fmt.Fprintln(&text, "\n- 新增依赖：无。")
	}
	return text.String()
}

func Stage(record ledger.PlanRecord) (publish func() error, cleanup func(), err error) {
	dir := filepath.Dir(record.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	file, err := os.CreateTemp(dir, ".plan-*.tmp")
	if err != nil {
		return nil, nil, err
	}
	name := file.Name()
	cleanup = func() { _ = os.Remove(name) }
	if _, err = file.WriteString(record.Content); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return func() error { return os.Rename(name, record.Path) }, cleanup, nil
}
