package webui

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

type ErrorResponse struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	FieldPaths []string `json:"field_paths"`
	GuardID    *string  `json:"guard_id"`
}
type RecoveryAdvice struct {
	Action    string `json:"action"`
	RetrySafe bool   `json:"retry_safe"`
	Message   string `json:"message"`
}

const RecoveryNone = "none"
