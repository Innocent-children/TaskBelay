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
	"github.com/Innocent-children/taskbelay/internal/domain"
	coreprotocol "github.com/Innocent-children/taskbelay/internal/protocol"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type SystemStatusProvider func() SystemStatusResponse

func NewAPI(service *application.Service, core *coreprotocol.Handler, status SystemStatusProvider) (http.Handler, error) {
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
		pageNumber, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page, e := taskSummaryPage(r.Context(), service.Store.List, store.ListQuery{Page: pageNumber, IncludeArchived: r.URL.Query().Get("include_archived") == "true", Status: r.URL.Query().Get("status"), Search: r.URL.Query().Get("search"), Host: domain.Host(r.URL.Query().Get("host"))}, r.URL.Query().Get("lifecycle"))
		if e != nil {
			http.Error(w, "invalid task page or unavailable store", 400)
			return
		}
		write(w, page)
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		page, e := service.Store.List(r.Context(), store.ListQuery{})
		if e != nil {
			http.Error(w, "task store unavailable", 503)
			return
		}
		counts, e := service.Store.StatusCounts(r.Context())
		if e != nil {
			http.Error(w, "task store unavailable", 503)
			return
		}
		recent := []map[string]any{}
		for _, t := range page.Items {
			s := coreprotocol.Summary(t)
			if len(recent) < 20 {
				recent = append(recent, s)
			}
		}
		write(w, map[string]any{"ok": true, "readiness": "ready", "counts": counts, "recent": recent})
	})
	mux.HandleFunc("GET /api/tasks/{task_id}", func(w http.ResponseWriter, r *http.Request) {
		refresh := r.URL.Query().Get("refresh") == "1"
		raw, _ := json.Marshal(coreprotocol.GetRequest{TaskID: r.PathValue("task_id"), Refresh: &refresh, Detail: true})
		result := core.Dispatch(r.Context(), "taskbelay_get_task", raw)
		if !result.OK {
			write(w, result)
			return
		}
		v := *result.Result.(coreprotocol.TaskResult).Details
		write(w, map[string]any{"ok": true, "readiness": "ready", "summary": coreprotocol.Summary(v.Task), "result": v})
	})
	mux.HandleFunc("POST /api/core/{tool}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CSRF      string          `json:"csrf"`
			Arguments json.RawMessage `json:"arguments"`
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
		if e != nil || coreprotocol.Decode(raw, &body) != nil {
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
		if coreprotocol.Decode(raw, &body) != nil || body.ConfirmTaskID != r.PathValue("task_id") {
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
		if !dispatched && r.Method != http.MethodGet {
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

// Desktop discovery filters the saved Core presentation before paging. A blocked
// task remains active in storage; the summary producer owns that classification.
func taskSummaryPage(ctx context.Context, list func(context.Context, store.ListQuery) (store.TaskPage, error), q store.ListQuery, lifecycle string) (map[string]any, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Page < 1 || q.Page > 1000000 {
		return nil, domain.Invalid("page", "page must be 1..1000000")
	}
	pageNumber := q.Page
	items := []map[string]any{}
	result := func(next bool) map[string]any {
		return map[string]any{"ok": true, "readiness": "ready", "page": pageNumber, "has_next": next, "items": items}
	}
	if lifecycle == "" {
		page, e := list(ctx, q)
		if e != nil {
			return nil, e
		}
		for _, t := range page.Items {
			items = append(items, coreprotocol.Summary(t))
		}
		return result(page.HasNext), nil
	}
	status := lifecycle
	if lifecycle == "blocked" {
		status = "active"
	}
	if status != "active" && status != "done" && status != "cancelled" {
		return nil, domain.Invalid("lifecycle", "unsupported task lifecycle filter")
	}
	if q.Status != "" && q.Status != status {
		return nil, domain.Invalid("status", "status conflicts with lifecycle filter")
	}
	q.Status = status
	q.Page = 1
	q.PageSize = 100
	skip := (pageNumber - 1) * 20
	for {
		page, e := list(ctx, q)
		if e != nil {
			return nil, e
		}
		for _, t := range page.Items {
			summary := coreprotocol.Summary(t)
			if summary["lifecycle"] != lifecycle {
				continue
			}
			if skip > 0 {
				skip--
				continue
			}
			if len(items) == 20 {
				return result(true), nil
			}
			items = append(items, summary)
		}
		if !page.HasNext {
			return result(false), nil
		}
		q.Page++
	}
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
