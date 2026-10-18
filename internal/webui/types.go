package webui

import "github.com/Innocent-children/taskbelay/internal/protocol"

type TaskDetailResponse struct {
	OK        bool                 `json:"ok" description:"Whether the requested Task was read successfully."`
	Readiness Readiness            `json:"readiness" description:"Local service readiness, not Task completion."`
	Summary   protocol.TaskSummary `json:"summary" description:"Same Core-owned saved summary used by task lists and desktop presentation."`
	Result    protocol.TaskResult  `json:"result" description:"Public get_task result with context and requested details, not an internal application snapshot."`
}

type Readiness string

const (
	ReadinessReady        Readiness = "ready"
	ReadinessReadOnly     Readiness = "read_only"
	ReadinessIncompatible Readiness = "incompatible"
	ReadinessUnavailable  Readiness = "unavailable"
)

type SystemStatusResponse struct {
	OK             bool      `json:"ok"`
	Readiness      Readiness `json:"readiness"`
	CoreIdentity   string    `json:"core_identity"`
	DataRootDigest string    `json:"data_root_digest"`
	URL            string    `json:"url"`
}

const maxRequestBodyBytes = 1024 * 1024
