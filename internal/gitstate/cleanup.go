package gitstate

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

/**
 * 清理只接受 TaskBelay 自己创建的无父提交快照。使用旧对象编号比较删除，
 * 引用被其他进程改动时保留，不触及分支、标签、真实索引或用户提交。
 */
func (w Workspace) ReleaseSnapshot(ctx context.Context, ref string) error {
	parts := strings.Split(ref, "/")
	if len(parts) != 4 || parts[0] != "refs" || parts[1] != "taskbelay" || !safeComponent(parts[2]) || !safeComponent(parts[3]) {
		return fmt.Errorf("不是 TaskBelay 快照引用: %s", ref)
	}
	out, err := w.command(ctx, nil, nil, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(objecttype)%00%(authorname)%00%(authoremail)%00%(subject)%00%(parent)", ref)
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\n"), "\x00")
	if len(fields) != 7 || fields[0] != ref || !validObjectID(fields[1]) || fields[2] != "commit" || fields[3] != "TaskBelay" || fields[4] != "<taskbelay@localhost>" || fields[5] != "TaskBelay snapshot" || fields[6] != "" {
		return fmt.Errorf("快照引用归属无法核对，保留: %s", ref)
	}
	_, err = w.command(ctx, nil, nil, "update-ref", "-d", ref, fields[1])
	return err
}

func (w Workspace) OldSnapshots(ctx context.Context, before time.Time) ([]string, error) {
	out, err := w.command(ctx, nil, nil, "for-each-ref", "--format=%(refname)%00%(committerdate:unix)%00%(authorname)%00%(authoremail)%00%(subject)%00%(parent)", "refs/taskbelay/")
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, line := range strings.Split(string(out), "\n") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 6 {
			continue
		}
		seconds, err := strconv.ParseInt(fields[1], 10, 64)
		if err == nil && time.Unix(seconds, 0).Before(before) && fields[2] == "TaskBelay" && fields[3] == "<taskbelay@localhost>" && fields[4] == "TaskBelay snapshot" && fields[5] == "" {
			refs = append(refs, fields[0])
		}
	}
	return refs, nil
}
