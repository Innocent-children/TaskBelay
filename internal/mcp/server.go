// Package mcp exposes the shared Core protocol through the MCP SDK.
package mcp

import (
	"context"
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/protocol"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct{ sdk *sdk.Server }

func NewServer(handler *protocol.Handler, version, instructions string) *Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "taskbelay", Version: version}, &sdk.ServerOptions{Instructions: instructions})
	for _, tool := range protocol.Catalog {
		destructive, openWorld := false, false
		server.AddTool(&sdk.Tool{Name: tool.Name, Description: tool.Description, InputSchema: protocol.Schema(tool.Input), OutputSchema: protocol.OutputSchema(tool.Output), Annotations: &sdk.ToolAnnotations{ReadOnlyHint: tool.ReadOnly, IdempotentHint: true, DestructiveHint: &destructive, OpenWorldHint: &openWorld}}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			raw := json.RawMessage(`{}`)
			if request != nil && request.Params != nil {
				raw = request.Params.Arguments
			}
			value := handler.Dispatch(ctx, tool.Name, raw)
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(encoded)}}, StructuredContent: json.RawMessage(encoded), IsError: !value.OK}, nil
		})
	}
	return &Server{sdk: server}
}
func (s *Server) Run(ctx context.Context, t sdk.Transport) error { return s.sdk.Run(ctx, t) }
