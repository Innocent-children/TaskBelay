package mcp

import (
	"context"
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/protocol"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransportDoesNotPublishExecution(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, _ := application.NewService(db, repository.NewGitObserver())
	server := NewServer(protocol.NewHandler(service, "0.25.0", userconfig.Preferences{}), "0.25.0", "", false)
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	session, err := server.sdk.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
	connected, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connected.Close()
	catalog, err := connected.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Tools) != len(protocol.Catalog) {
		t.Fatal("catalog mismatch")
	}
	for _, tool := range catalog.Tools {
		if tool.Name == "taskbelay_run_check" || tool.Name == "check.run" {
			t.Fatal("execution published")
		}
	}
	result, err := connected.CallTool(ctx, &sdk.CallToolParams{Name: "taskbelay_server_info", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal(err, result)
	}
}

func TestSummaryTextPreservesCanonicalSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, _ := application.NewService(db, repository.NewGitObserver())
	server := NewServer(protocol.NewHandler(service, "0.26.0", userconfig.Preferences{}), "0.26.0", "", true)
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.sdk.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "fixture", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, args := range []map[string]any{{}, {"unexpected": true}} {
		result, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "taskbelay_server_info", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var envelope protocol.Envelope
		if err = json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.OK == result.IsError || envelope.Tool != "taskbelay_server_info" {
			t.Fatal(string(raw))
		}
		if len(result.Content) != 1 {
			t.Fatal(result)
		}
		message := result.Content[0].(*sdk.TextContent).Text
		if strings.Contains(message, `"result"`) || len(message) > 350 {
			t.Fatal("summary repeated the full payload", message)
		}
		if !envelope.OK && (envelope.Error == nil || envelope.Recovery == nil || envelope.Outcome != "not_applied") {
			t.Fatal("failure lost recovery", string(raw))
		}
	}
}
