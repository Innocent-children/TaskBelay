package webui

import (
	"context"
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	core "github.com/Innocent-children/taskbelay/internal/protocol"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSessionAndCoreProjection(t *testing.T) {
	db, e := store.Open(context.Background(), filepath.Join(t.TempDir(), "taskbelay.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	service, _ := application.NewService(db, repository.NewGitObserver())
	m := core.NewHandler(service, "0.25.0", userconfig.Preferences{})
	api, _ := NewAPI(service, m, func() SystemStatusResponse {
		return SystemStatusResponse{Readiness: ReadinessReady, CoreIdentity: "current"}
	})
	session, _ := NewSession()
	handler := protectMutations("http://127.0.0.1:1234", session, api)
	r := httptest.NewRequest("POST", "http://127.0.0.1:1234/api/core/taskbelay_server_info", strings.NewReader(`{"csrf":"wrong","arguments":{}}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("untrusted mutation allowed")
	}
	body, _ := json.Marshal(map[string]any{"csrf": session.Value(), "arguments": map[string]any{}})
	r = httptest.NewRequest("POST", "http://127.0.0.1:1234/api/core/taskbelay_server_info", strings.NewReader(string(body)))
	r.Header.Set("Origin", "http://127.0.0.1:1234")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	var v core.Envelope
	if json.Unmarshal(w.Body.Bytes(), &v) != nil || !v.OK || v.Tool != "taskbelay_server_info" {
		t.Fatal(w.Body.String())
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:1234/api/tasks", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal(w.Body.String())
	}
}
func TestExactHostRejectsAlternateAddress(t *testing.T) {
	h := exactHost("127.0.0.1:1234", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:1234/", nil))
	if w.Code != 403 {
		t.Fatal("alternate host accepted")
	}
}

func TestDesktopLifecycleFilteringPrecedesPagination(t *testing.T) {
	approved := domain.Task{TaskID: "approved", Status: "active", Repositories: []domain.RepositoryScopeEntry{{Key: "primary"}}}
	blocked := approved
	blocked.TaskID = "blocked"
	blocked.Issue = &domain.Issue{Kind: "authorization", Summary: "User decision needed", Options: []string{"Keep scope", "Expand scope"}}
	list := func(_ context.Context, q store.ListQuery) (store.TaskPage, error) {
		if q.Status != "active" || q.PageSize != 100 {
			t.Fatal("discovery must restrict storage to active tasks", q)
		}
		items := []domain.Task{}
		if q.Page == 1 {
			for i := 0; i < 100; i++ {
				items = append(items, approved)
			}
		} else {
			for i := 0; i < 21; i++ {
				items = append(items, blocked)
			}
		}
		return store.TaskPage{Items: items, Page: q.Page, HasNext: q.Page == 1}, nil
	}
	first, e := taskSummaryPage(context.Background(), list, store.ListQuery{Page: 1}, "blocked")
	if e != nil || len(first["items"].([]core.TaskSummary)) != 20 || first["has_next"] != true {
		t.Fatal("older blocked tasks disappeared", first, e)
	}
	second, e := taskSummaryPage(context.Background(), list, store.ListQuery{Page: 2}, "blocked")
	if e != nil || len(second["items"].([]core.TaskSummary)) != 1 || second["has_next"] != false {
		t.Fatal("filtered pagination is incorrect", second, e)
	}
}
