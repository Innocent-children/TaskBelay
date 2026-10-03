package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOutputSchemasAcceptCurrentTaskAndActionProjections(t *testing.T) {
	for _, node := range []domain.NodeID{domain.NodeRequirements, domain.NodeDesign, domain.NodeTasks, domain.NodeImplement, domain.NodeTest, domain.NodeComprehensionReview, domain.NodeRefactor, domain.NodeDelivery, domain.NodeBlocked, domain.NodeDone, domain.NodeCancelled} {
		t.Run(string(node), func(t *testing.T) {
			var action *domain.ProcessAction
			if !node.Terminal() {
				value, err := workflow.BuildProcessActionForWorkspace(workflow.StandardProcess(), node, "task", 1, methodProjectionWorkspace(), domain.MethodPlain, "action", time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				action = &value
			}
			task := domain.ProcessTask{TaskID: "task", Revision: 1, CurrentNode: node, CurrentAction: action}
			for _, definition := range ToolCatalog() {
				var result any = projectTask(task)
				switch definition.Name {
				case ToolServerInfo:
					result = ServerInfoResult{Product: "taskbelay"}
				case ToolOpenTask:
					result = map[string]any{"created": false, "task": projectTask(task), "recovery_assessment": nil}
				case ToolGetTask:
					result = map[string]any{"task": projectTask(task), "recovery_assessment": nil}
				case ToolGetNextAction:
					result = projectNextAction(application.NextActionResult{TaskID: task.TaskID, Revision: task.Revision, CurrentNode: node, Action: action})
				case ToolPrepareTaskRelocation:
					result = map[string]any{"task": projectTask(task), "relocation_id": "relocation"}
				case ToolPrepareTaskBranchRename:
					result = map[string]any{"task": projectTask(task), "rename_id": "rename"}
				}
				var schema jsonschema.Schema
				if err := json.Unmarshal(definition.OutputSchema, &schema); err != nil {
					t.Fatal(err)
				}
				resolved, err := schema.Resolve(nil)
				if err != nil {
					t.Fatal(err)
				}
				encoded := EncodeSuccess("request", definition.Name, result)
				var value any
				if err := json.Unmarshal(encoded.JSON, &value); err != nil {
					t.Fatal(err)
				}
				if err := resolved.Validate(value); err != nil {
					t.Fatalf("%s: %v", definition.Name, err)
				}
			}
		})
	}
}

func TestSDKPublishesOutputSchemasInstructionsAndStructuredResults(t *testing.T) {
	service, err := application.NewService(annotationStore{}, annotationObserver{})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(service, "test", &ServerOptions{Instructions: "Read operation help before constructing workspace arguments."})
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Run(ctx, serverTransport)
	client := sdk.NewClient(&sdk.Implementation{Name: "output-contract-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.InitializeResult().Instructions != "Read operation help before constructing workspace arguments." {
		t.Fatal("missing server instructions")
	}
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.OutputSchema == nil {
			t.Fatalf("%s has no output schema", tool.Name)
		}
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: ToolServerInfo, Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || result.StructuredContent == nil {
		t.Fatalf("result=%#v", result)
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var fromText, fromStructured any
	if err := json.Unmarshal([]byte(result.Content[0].(*sdk.TextContent).Text), &fromText); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(structured, &fromStructured); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromText, fromStructured) {
		t.Fatal("structured and text results differ")
	}
}

func TestSDKFailureRetainsStructuredEnvelopeWithoutRequestIdentity(t *testing.T) {
	service, err := application.NewService(annotationStore{}, annotationObserver{})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(service, "test", &ServerOptions{NewRequestID: func() (domain.ID, error) { return "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Run(ctx, serverTransport)
	client := sdk.NewClient(&sdk.Implementation{Name: "output-failure-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: ToolServerInfo, Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.StructuredContent == nil {
		t.Fatalf("result=%#v", result)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var structured, textual any
	if err := json.Unmarshal(raw, &structured); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*sdk.TextContent).Text), &textual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(structured, textual) {
		t.Fatal("failure envelopes differ")
	}
}
