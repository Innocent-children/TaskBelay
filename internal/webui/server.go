package webui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	coremcp "github.com/Innocent-children/taskbelay/internal/mcp"
)

type SystemStatusProvider func() SystemStatusResponse

func NewAPI(service *application.Service, core *coremcp.Server, status SystemStatusProvider) (http.Handler, error) {
	if service == nil || core == nil || status == nil {
		return nil, ErrSessionUnavailable
	}
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/system/status", func(w http.ResponseWriter, r *http.Request) { s := status(); s.OK = true; write(w, s) })
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		ts, e := service.Store.List(r.Context())
		if e != nil {
			http.Error(w, "task store unavailable", 503)
			return
		}
		items := []map[string]any{}
		for _, t := range ts {
			s := coremcp.Summary(t)
			filter := r.URL.Query().Get("lifecycle")
			if filter == "" || s["lifecycle"] == filter {
				items = append(items, s)
			}
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		start := (page - 1) * 20
		if start > len(items) {
			start = len(items)
		}
		end := start + 20
		if end > len(items) {
			end = len(items)
		}
		write(w, map[string]any{"ok": true, "readiness": "ready", "page": page, "has_next": end < len(items), "items": items[start:end]})
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		ts, e := service.Store.List(r.Context())
		if e != nil {
			http.Error(w, "task store unavailable", 503)
			return
		}
		counts := map[string]int{}
		recent := []map[string]any{}
		for _, t := range ts {
			s := coremcp.Summary(t)
			counts[s["lifecycle"].(string)]++
			if len(recent) < 20 {
				recent = append(recent, s)
			}
		}
		write(w, map[string]any{"ok": true, "readiness": "ready", "counts": counts, "recent": recent})
	})
	mux.HandleFunc("GET /api/tasks/{task_id}", func(w http.ResponseWriter, r *http.Request) {
		refresh := r.URL.Query().Get("refresh") == "1"
		raw, _ := json.Marshal(coremcp.GetRequest{TaskID: r.PathValue("task_id"), Refresh: &refresh})
		result := core.Dispatch(r.Context(), "taskbelay_get_task", raw)
		if !result.OK {
			write(w, result)
			return
		}
		v := result.Result.(application.View)
		write(w, map[string]any{"ok": true, "readiness": "ready", "summary": coremcp.Summary(v.Task), "result": v})
	})
	mux.HandleFunc("POST /api/core/{tool}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CSRF      string          `json:"csrf"`
			Arguments json.RawMessage `json:"arguments"`
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
		if e != nil || coremcp.Decode(raw, &body) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		write(w, core.Dispatch(r.Context(), r.PathValue("tool"), body.Arguments))
	})
	mux.HandleFunc("POST /api/tasks/{task_id}/purge", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CSRF          string `json:"csrf"`
			Revision      uint64 `json:"revision"`
			ConfirmTaskID string `json:"confirm_task_id"`
		}
		raw, _ := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
		if coremcp.Decode(raw, &body) != nil || body.ConfirmTaskID != r.PathValue("task_id") {
			http.Error(w, "exact terminal task confirmation is required", 400)
			return
		}
		if e := service.Store.Purge(r.Context(), body.ConfirmTaskID, body.Revision); e != nil {
			http.Error(w, e.Error(), 409)
			return
		}
		write(w, map[string]any{"ok": true})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Task views and tool endpoints already reserve their call in Dispatch.
		dispatched := strings.HasPrefix(r.URL.Path, "/api/core/") || r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/tasks/")
		if !dispatched {
			complete, e := service.MeasureControl(r.Context())
			if e != nil {
				http.Error(w, "control cost reservation unavailable", 503)
				return
			}
			defer complete()
		}
		mux.ServeHTTP(w, r)
	}), nil
}

type Server struct {
	listener net.Listener
	server   *http.Server
	origin   string
	session  Session
}

func NewServer(api http.Handler) (*Server, error) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	session, e := NewSession()
	if e != nil {
		l.Close()
		return nil, e
	}
	static, e := newStaticHandler(session.Value())
	if e != nil {
		l.Close()
		return nil, e
	}
	origin := "http://" + l.Addr().String()
	mux := http.NewServeMux()
	mux.Handle("/api/", protectMutations(origin, session, api))
	mux.Handle("/", static)
	return &Server{listener: l, origin: origin, session: session, server: &http.Server{Handler: exactHost(l.Addr().String(), mux), ReadHeaderTimeout: 5 * time.Second}}, nil
}
func (s *Server) URL() string { return s.origin }
func (s *Server) Serve() error {
	e := s.server.Serve(s.listener)
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
func (s *Server) Close(ctx context.Context) error { return s.server.Shutdown(ctx) }
func exactHost(host string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "exact loopback host required", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}
