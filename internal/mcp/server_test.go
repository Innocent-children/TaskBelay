package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, e := store.Open(context.Background(), filepath.Join(t.TempDir(), "taskbelay.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	a, _ := application.NewService(s, repository.NewGitObserver())
	v, e := NewServer(a, "0.20.0", nil)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func validate(t *testing.T, schema map[string]any, v any) {
	t.Helper()
	raw, _ := json.Marshal(schema)
	var definition jsonschema.Schema
	if e := json.Unmarshal(raw, &definition); e != nil {
		t.Fatal(e)
	}
	resolved, e := definition.Resolve(nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(v)
	var instance any
	json.Unmarshal(raw, &instance)
	if e = resolved.Validate(instance); e != nil {
		t.Fatalf("schema mismatch: %v\n%s", e, raw)
	}
}
func TestEveryToolEncodesActualErrorAgainstPublishedSchema(t *testing.T) {
	s := testServer(t)
	for _, tool := range Catalog {
		t.Run(tool.Name, func(t *testing.T) {
			actual := s.Dispatch(context.Background(), tool.Name, []byte(`{"unexpected":true}`))
			actual.RequestID = "example-request"
			if actual.OK {
				t.Fatal("unknown member accepted")
			}
			expected := Envelope{RequestID: "example-request", Tool: tool.Name, Error: &domain.Error{Code: "INVALID_ARGUMENT", Message: "member is not allowed by the current request", Details: []domain.ContractViolation{{Path: "arguments.unexpected", Rule: "requirement", Message: "member is not allowed by the current request"}}}, Recovery: &Recovery{Action: "correct_request", RetrySafe: false, Message: "Correct only the identified fields from existing facts; preserve operation identity and real authorization.", AllowedPaths: []string{"arguments.unexpected"}}}
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(expected)
			if string(a) != string(b) {
				t.Fatalf("encoded error differs\n%s\n%s", a, b)
			}
			validate(t, OutputSchema(tool.Output), actual)
		})
	}
}

func TestNestedRequestErrorsIdentifyTheExactMember(t *testing.T) {
	s := testServer(t)
	for _, c := range []struct{ raw, path, message string }{
		{`{"authorization":{"goals":[{"unexpected":true}]}}`, "arguments.authorization.goals[0].unexpected", "member is not allowed by the current request"},
		{`{"authorization":{"goals":[{"statement":2}]}}`, "arguments.authorization.goals[0].statement", "requires string"},
		{`{"checks":[{"argv":[true]}]}`, "arguments.checks[0].argv[0]", "requires string"},
		{`{"checks":[{"id":"a","id":"b"}]}`, "arguments.checks[0].id", "duplicate JSON member"},
	} {
		actual := s.Dispatch(context.Background(), "taskbelay_open_task", []byte(c.raw))
		if actual.OK || actual.Error.Code != "INVALID_ARGUMENT" || len(actual.Error.Details) != 1 || actual.Error.Details[0].Path != c.path || actual.Error.Message != c.message {
			t.Fatalf("incorrect field error for %s: %+v", c.raw, actual)
		}
		validate(t, OutputSchema(application.View{}), actual)
		if tasks, e := s.service.Store.List(context.Background()); e != nil || len(tasks) != 0 {
			t.Fatal("invalid input created semantic state", e)
		}
	}
}
func TestSDKTransportPublishesOnlyCurrentToolsAndRealEnvelopes(t *testing.T) {
	s := testServer(t)
	left, right := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, e := s.sdk.Connect(ctx, left, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "contract-test", Version: "1"}, nil)
	connection, e := client.Connect(ctx, right, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer connection.Close()
	tools, e := connection.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(tools.Tools) != len(Catalog) {
		t.Fatal("catalog mismatch")
	}
	r, e := connection.CallTool(ctx, &sdk.CallToolParams{Name: "taskbelay_server_info", Arguments: map[string]any{}})
	if e != nil {
		t.Fatal(e)
	}
	if r.IsError {
		t.Fatal(r)
	}
	raw := r.Content[0].(*sdk.TextContent).Text
	var v Envelope
	if json.Unmarshal([]byte(raw), &v) != nil || !v.OK || v.Tool != "taskbelay_server_info" {
		t.Fatal("invalid actual SDK response")
	}
	validate(t, OutputSchema(Catalog[0].Output), v)
}
func TestRequestDecodeRejectsDuplicatesTypesAndTrailingValues(t *testing.T) {
	for _, raw := range []string{`{"task_id":"x","task_id":"y"}`, `{"task_id":7}`, `{} {}`, `[]`, "{\"task_id\":\"\xff\"}"} {
		var q GetRequest
		if e := Decode([]byte(raw), &q); e == nil {
			t.Fatal("invalid request accepted", raw)
		}
	}
	var q GetRequest
	if e := Decode([]byte(`{"task_id":"x"}`), &q); e != nil {
		t.Fatal(e)
	}
}
func TestCompleteSkillErrorExampleMatchesActualProducer(t *testing.T) {
	s := testServer(t)
	actual := s.Dispatch(context.Background(), "taskbelay_server_info", []byte(`{"unexpected":true}`))
	actual.RequestID = "example-request"
	b, _ := json.Marshal(actual)
	p := filepath.Join("..", "..", "skills", "taskbelay", "core", "protocol.md")
	doc, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(doc), string(b)) {
		t.Fatal("shared complete error example differs from the producer")
	}
}
