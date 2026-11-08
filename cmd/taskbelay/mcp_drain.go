package main

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

/**
 * SDK 的 Session.Close 会阻止后续响应写出。因此部署时先拒绝新调用，
 * 让已交给 SDK 的请求写完响应，再关闭 stdio。取消通知仍可送达。
 */
type retiringMCP struct {
	transport mcp.Transport
	mcp.Connection
	mu      sync.Mutex
	pending int
	drained chan struct{}
}

func (t *retiringMCP) Connect(ctx context.Context) (mcp.Connection, error) {
	conn, err := t.transport.Connect(ctx)
	if err != nil {
		return nil, err
	}
	t.Connection = conn
	return t, nil
}

func (t *retiringMCP) retire() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drained = make(chan struct{})
	if t.pending == 0 {
		close(t.drained)
	}
	return t.drained
}

func (t *retiringMCP) Read(ctx context.Context) (jsonrpc.Message, error) {
	for {
		msg, err := t.Connection.Read(ctx)
		req, ok := msg.(*jsonrpc.Request)
		if err != nil || !ok || !req.IsCall() {
			return msg, err
		}
		t.mu.Lock()
		accept := t.drained == nil
		if accept {
			t.pending++
		}
		t.mu.Unlock()
		if accept {
			return msg, nil
		}
		err = t.Connection.Write(ctx, &jsonrpc.Response{ID: req.ID, Error: &jsonrpc.Error{
			Code: jsonrpc.CodeInternalError, Message: "TaskBelay 已更新，请重连 MCP。",
		}})
		if err != nil {
			return nil, err
		}
	}
}

func (t *retiringMCP) Write(ctx context.Context, msg jsonrpc.Message) error {
	err := t.Connection.Write(ctx, msg)
	if _, ok := msg.(*jsonrpc.Response); ok {
		t.mu.Lock()
		t.pending--
		if t.drained != nil && t.pending == 0 {
			close(t.drained)
		}
		t.mu.Unlock()
	}
	return err
}
