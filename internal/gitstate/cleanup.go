package gitstate

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SnapshotRef struct{ Ref, Object, Type, WorkspaceID string }

func (ref SnapshotRef) Validate() error {
	parts := strings.Split(ref.Ref, "/")
	if len(parts) != 4 || parts[0] != "refs" || parts[1] != "taskbelay" || !safeComponent(parts[2]) || !safeComponent(parts[3]) || !validObjectID(ref.Object) || (ref.Type != "tree" && ref.Type != "commit") || ref.WorkspaceID == "" {
		return fmt.Errorf("快照引用身份无效: %s", ref.Ref)
	}
	return nil
}

func (w Workspace) snapshotFields(ctx context.Context, ref string) ([]string, error) {
	out, err := w.command(ctx, nil, nil, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(objecttype)%00%(authorname)%00%(authoremail)%00%(subject)%00%(parent)%00%(symref)", ref)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\x00"), nil
}

/**
 * 队列提供工作区及预期对象，旧观察只允许释放已核实的无父快照 commit。
 * --no-deref 和旧对象比较共同避免跟随符号引用或删除后来替换的对象。
 */
func (w Workspace) ReleaseSnapshot(ctx context.Context, ref SnapshotRef) error {
	parts := strings.Split(ref.Ref, "/")
	if ref.WorkspaceID != w.ID || len(parts) != 4 || parts[0] != "refs" || parts[1] != "taskbelay" || !safeComponent(parts[2]) || !safeComponent(parts[3]) {
		return fmt.Errorf("快照引用归属不符: %s", ref.Ref)
	}
	fields, err := w.snapshotFields(ctx, ref.Ref)
	if err != nil {
		return err
	}
	if len(fields) == 0 {
		return nil
	}
	if len(fields) != 8 || fields[0] != ref.Ref || !validObjectID(fields[1]) || fields[7] != "" {
		return fmt.Errorf("快照引用不是可核对的直接引用: %s", ref.Ref)
	}
	if ref.Object != "" && fields[1] != ref.Object {
		return fmt.Errorf("快照引用对象已改变，保留: %s", ref.Ref)
	}
	if ref.Type != "" && fields[2] != ref.Type {
		return fmt.Errorf("快照引用类型不符: %s", ref.Ref)
	}
	switch fields[2] {
	case "tree":
		if ref.Object == "" {
			return fmt.Errorf("tree 引用缺少预登记对象: %s", ref.Ref)
		}
	case "commit":
		if fields[3] != "TaskBelay" || fields[4] != "<taskbelay@localhost>" || fields[5] != "TaskBelay snapshot" || fields[6] != "" {
			return fmt.Errorf("旧快照提交归属无法核对: %s", ref.Ref)
		}
	default:
		return fmt.Errorf("快照引用类型无效: %s", ref.Ref)
	}
	_, err = w.command(ctx, nil, nil, "update-ref", "--no-deref", "-d", ref.Ref, fields[1])
	return err
}

func (w Workspace) OldSnapshots(ctx context.Context, before time.Time) ([]SnapshotRef, error) {
	out, err := w.command(ctx, nil, nil, "for-each-ref", "--format=%(refname)%00%(committerdate:unix)%00%(authorname)%00%(authoremail)%00%(subject)%00%(parent)%00%(objectname)%00%(objecttype)%00%(symref)", "refs/taskbelay/")
	if err != nil {
		return nil, err
	}
	var refs []SnapshotRef
	for _, line := range strings.Split(string(out), "\n") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 9 {
			continue
		}
		seconds, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || !time.Unix(seconds, 0).Before(before) || fields[2] != "TaskBelay" || fields[3] != "<taskbelay@localhost>" || fields[4] != "TaskBelay snapshot" || fields[5] != "" || fields[7] != "commit" || fields[8] != "" {
			continue
		}
		ref := SnapshotRef{Ref: fields[0], Object: fields[6], Type: "commit", WorkspaceID: w.ID}
		if ref.Validate() == nil {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
