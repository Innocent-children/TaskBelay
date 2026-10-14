package mcp

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/protocol"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"
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
	server := NewServer(protocol.NewHandler(service, "0.25.0", userconfig.Preferences{}), "0.25.0", "")
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
