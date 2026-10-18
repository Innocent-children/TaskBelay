package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	coreprotocol "github.com/Innocent-children/taskbelay/internal/protocol"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
)

const databaseFileName = "taskbelay.db"

type RuntimeState struct {
	Readiness      Readiness `json:"readiness"`
	CoreIdentity   string    `json:"core_identity"`
	DataRootDigest string    `json:"data_root_digest"`
	URL            string    `json:"url"`
	PID            int       `json:"pid,omitempty"`
}

func Start(ctx context.Context, dataDirectory, coreIdentity string, noOpen bool) (RuntimeState, error) {
	state, _ := Status(ctx, dataDirectory, coreIdentity)
	if state.Readiness == ReadinessReady {
		if !noOpen {
			if err := OpenBrowser(state.URL); err != nil {
				return state, err
			}
		}
		return state, nil
	}
	if state.Readiness == ReadinessIncompatible {
		return state, fmt.Errorf("WebUI is %s", state.Readiness)
	}
	executable, err := os.Executable()
	if err != nil {
		return RuntimeState{}, err
	}
	logPath := filepath.Join(dataDirectory, "webui-runtime.log")
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return RuntimeState{}, err
	}
	command := exec.Command(executable, "webui", "serve")
	command.Env = os.Environ()
	command.Stdin = nil
	command.Stdout = logFile
	command.Stderr = logFile
	configureBackgroundCommand(command)
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return RuntimeState{}, err
	}
	_ = logFile.Close()
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		// The child owns storage initialization until it publishes its runtime receipt.
		state, _ = runtimeStatus(ctx, dataDirectory, coreIdentity, false)
		if state.Readiness == ReadinessReady {
			if !noOpen {
				if err := OpenBrowser(state.URL); err != nil {
					return state, err
				}
			}
			return state, nil
		}
		if state.Readiness == ReadinessIncompatible {
			return state, fmt.Errorf("WebUI is %s", state.Readiness)
		}
		select {
		case exitError := <-exited:
			return RuntimeState{}, fmt.Errorf("WebUI process exited before readiness (%v); inspect %s", exitError, logPath)
		case <-ctx.Done():
			return RuntimeState{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return RuntimeState{}, fmt.Errorf("WebUI did not become ready; inspect %s before retrying the original startup", logPath)
}

func Status(ctx context.Context, dataDirectory, coreIdentity string) (RuntimeState, error) {
	return runtimeStatus(ctx, dataDirectory, coreIdentity, true)
}

func runtimeStatus(ctx context.Context, dataDirectory, coreIdentity string, inspectStoppedStorage bool) (RuntimeState, error) {
	digest, err := DataRootDigest(dataDirectory)
	if err != nil {
		return RuntimeState{Readiness: ReadinessUnavailable, CoreIdentity: coreIdentity}, err
	}
	base := RuntimeState{Readiness: ReadinessUnavailable, CoreIdentity: coreIdentity, DataRootDigest: digest}
	receipt, err := ReadReceipt(dataDirectory)
	if errors.Is(err, os.ErrNotExist) {
		if inspectStoppedStorage {
			base = classifyStoppedStorage(ctx, dataDirectory, base)
		}
		return base, nil
	}
	if err != nil {
		return base, err
	}
	base.URL = receipt.URL
	base.PID = receipt.PID
	if !receiptProcessMatches(receipt) {
		_ = removeReceipt(dataDirectory, receipt)
		if inspectStoppedStorage {
			base = classifyStoppedStorage(ctx, dataDirectory, base)
		}
		return base, nil
	}
	if receipt.DataRootDigest != digest {
		base.Readiness = ReadinessIncompatible
		return base, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, receipt.URL+"/api/system/status", nil)
	if err != nil {
		return base, err
	}
	client := &http.Client{Timeout: 750 * time.Millisecond}
	response, err := client.Do(request)
	if err != nil {
		return base, nil
	}
	defer response.Body.Close()
	var live SystemStatusResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK || decoder.Decode(&live) != nil || live.CoreIdentity != coreIdentity || live.DataRootDigest != digest || live.URL != receipt.URL {
		base.Readiness = ReadinessIncompatible
		return base, nil
	}
	base.Readiness = live.Readiness
	return base, nil
}

func classifyStoppedStorage(ctx context.Context, dataDirectory string, state RuntimeState) RuntimeState {
	databasePath := filepath.Join(dataDirectory, databaseFileName)
	if _, err := os.Stat(databasePath); errors.Is(err, os.ErrNotExist) {
		return state
	}
	taskStore, err := store.Open(ctx, databasePath)
	if err == nil {
		_ = taskStore.Close()
		return state
	}
	var failure *domain.Error
	if errors.As(err, &failure) && (failure.Code == "SCHEMA_UNSUPPORTED" || failure.Code == "STORAGE_CORRUPT") {
		state.Readiness = ReadinessIncompatible
		return state
	}
	if file, openErr := os.Open(databasePath); openErr == nil {
		_ = file.Close()
		state.Readiness = ReadinessReadOnly
	}
	return state
}

func OpenBrowser(url string) error {
	if url == "" {
		return fmt.Errorf("WebUI URL is unavailable")
	}
	return openBrowser(url)
}

func Stop(ctx context.Context, dataDirectory, coreIdentity string) (RuntimeState, error) {
	state, err := Status(ctx, dataDirectory, coreIdentity)
	if err != nil || state.Readiness != ReadinessReady {
		return state, err
	}
	receipt, err := ReadReceipt(dataDirectory)
	if err != nil || !receiptProcessMatches(receipt) {
		return state, fmt.Errorf("WebUI process identity changed")
	}
	process, err := os.FindProcess(receipt.PID)
	if err != nil {
		return state, err
	}
	if err := signalWebUIProcess(process, receipt.PID); err != nil {
		if !receiptProcessMatches(receipt) {
			_ = removeReceipt(dataDirectory, receipt)
			state.Readiness = ReadinessUnavailable
			return state, nil
		}
		return state, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !receiptProcessMatches(receipt) {
			_ = removeReceipt(dataDirectory, receipt)
			state.Readiness = ReadinessUnavailable
			return state, nil
		}
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	forced, forceErr := forceStopWebUIProcess(process)
	if forceErr != nil {
		if !receiptProcessMatches(receipt) {
			_ = removeReceipt(dataDirectory, receipt)
			state.Readiness = ReadinessUnavailable
			return state, nil
		}
		return state, forceErr
	}
	if forced {
		deadline = time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if !receiptProcessMatches(receipt) {
				_ = removeReceipt(dataDirectory, receipt)
				state.Readiness = ReadinessUnavailable
				return state, nil
			}
			select {
			case <-ctx.Done():
				return state, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	return state, fmt.Errorf("WebUI did not stop")
}

func Serve(ctx context.Context, dataDirectory, coreIdentity string) error {
	digest, err := DataRootDigest(dataDirectory)
	if err != nil {
		return err
	}
	databasePath := filepath.Join(dataDirectory, databaseFileName)
	taskStore, err := store.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer taskStore.Close()
	service, err := application.NewService(taskStore, repository.NewGitObserver())
	if err != nil {
		return err
	}
	var server *Server
	status := func() SystemStatusResponse {
		url := ""
		if server != nil {
			url = server.URL()
		}
		return SystemStatusResponse{Readiness: ReadinessReady, CoreIdentity: coreIdentity, DataRootDigest: digest, URL: url}
	}
	core := coreprotocol.NewHandler(service, coreIdentity, userconfig.Preferences{})
	api, err := NewAPI(service, core, status)
	if err != nil {
		return err
	}
	server, err = NewServer(api)
	if err != nil {
		return err
	}
	defer server.Close(context.Background())
	identity, err := processStartIdentity(os.Getpid())
	if err != nil {
		return err
	}
	receipt := RuntimeReceipt{PID: os.Getpid(), ProcessStartIdentity: identity, DataRootDigest: digest, URL: server.URL(), CreatedAt: time.Now().UTC()}
	if err := writeReceipt(dataDirectory, receipt); err != nil {
		return err
	}
	defer removeReceipt(dataDirectory, receipt)
	serveError := make(chan error, 1)
	go func() { serveError <- server.Serve() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Close(shutdownCtx)
	case err := <-serveError:
		return err
	}
}
