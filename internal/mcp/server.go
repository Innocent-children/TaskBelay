// Package mcp exposes the shared Core protocol through the MCP SDK.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/protocol"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct{ sdk *sdk.Server }

func NewServer(handler *protocol.Handler, version, instructions string, summaryText bool) *Server {
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
			text := string(encoded)
			if summaryText {
				text = fmt.Sprintf("%s: ok=%t. Read the complete response in structuredContent.", tool.Name, value.OK)
				if value.Error != nil {
					text += " " + value.Error.Code + ": " + value.Error.Message
				}
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}, StructuredContent: json.RawMessage(encoded), IsError: !value.OK}, nil
		})
	}
	return &Server{sdk: server}
}
func (s *Server) Run(ctx context.Context, t sdk.Transport) error { return s.sdk.Run(ctx, t) }
