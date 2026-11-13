package ledger

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestCheckRulesUpgradeValidity(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, t.TempDir(), NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.db.ExecContext(ctx, `
INSERT INTO workspaces_v12(id,project_id,path) VALUES('w','p','repo');
INSERT INTO observations_v12(id,workspace_id,task_id,data)
VALUES(1,'w','t1','{"WorkspaceID":"w","Tree":"tree","Full":true,"Complete":true}');
INSERT INTO tasks_v12(id,workspace_id,state,branch,data)
VALUES('t1','w','active','main','{"id":"t1","workspace_id":"w","last_observation_id":1}');`)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.CheckInput(ctx, "w", "t1", Actor{}, "algorithm")
	if err != nil {
		t.Fatal(err)
	}
	previous := current
	previous.RulesVersion = 3
	for _, sample := range []struct {
		input CheckInput
		want  string
	}{{previous, "expired"}, {current, "current"}} {
		// 还原升级前已经完成的记录；SaveCheck 会按运行中的规则重新判定状态。
		record := CheckRecord{Input: sample.input, InputVersion: sample.input.Version(), Depth: "static", Status: CheckComplete}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		row, err := store.db.ExecContext(ctx, `INSERT INTO checks_v12(task_id,depth,input_version,status,data)
VALUES(?,?,?,?,?)`, "t1", record.Depth, record.InputVersion, record.Status, string(data))
		if err != nil {
			t.Fatal(err)
		}
		id, err := row.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		page, err := store.TaskRecords(ctx, "w", Actor{}, RecordQuery{TaskID: "t1", Category: "checks", ID: strconv.FormatInt(id, 10)})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Records) != 1 {
			t.Fatalf("check %d: got %d records", id, len(page.Records))
		}
		if got := page.Records[0].State["validity"]; got != sample.want {
			t.Errorf("rules v%d: got validity %v, want %s", sample.input.RulesVersion, got, sample.want)
		}
	}
}
