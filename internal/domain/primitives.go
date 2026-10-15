// Package domain owns platform-neutral authorization, evidence and control rules.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type ID string

func (v ID) IsValid() bool {
	return v != "" && len(v) <= 128 && strings.TrimSpace(string(v)) == string(v) && !strings.ContainsAny(string(v), " \t\r\n\x00/\\")
}

type Digest string

func (v Digest) IsValid() bool {
	b, e := hex.DecodeString(string(v))
	return e == nil && len(b) == 32 && strings.ToLower(string(v)) == string(v)
}
func Hash(v any) Digest         { b, _ := json.Marshal(v); return HashBytes(b) }
func HashBytes(b []byte) Digest { h := sha256.Sum256(b); return Digest(hex.EncodeToString(h[:])) }

type Host string

func (v Host) IsValid() bool {
	return v == "codex" || v == "deepseek" || v == "claude" || v == "zcode" || v == "cli"
}

const (
	MaxRepositoryPathBytes    = 4096
	MaxRepositoryKeyBytes     = 128
	MaxAdditionalRepositories = 7
	MaxIdentifierBytes        = 128
	MaxFingerprintPaths       = 1024
	MaxInputFiles             = 16384
	MaxGitCommandOutputBytes  = 1024 * 1024
	GitCommandTimeout         = 10 * time.Second
	repositoryPathSeparator   = "::"
)

type ViolationRule string

func (v ViolationRule) Message() string { return "must be a canonical identifier" }

const (
	RuleEnumValueInvalid  ViolationRule = "enum_value_invalid"
	RuleIdentifierInvalid ViolationRule = "identifier_invalid"
	RuleMemberDependency  ViolationRule = "member_dependency"
	RuleValueFormat       ViolationRule = "value_format"
)

type ContractViolation struct {
	Path    string        `json:"path" description:"输入错误用arguments路径；审查/缺口使用限定仓库路径或具体记录位置。"`
	Rule    ViolationRule `json:"rule" description:"违反的字段/关系规则类别，不作为用户授权。"`
	Message string        `json:"message" description:"可直接阅读的具体原因，不含敏感原始参数或内部堆栈。"`
}

func ExplainedViolation(p string, r ViolationRule, m string) ContractViolation {
	return ContractViolation{p, r, m}
}

type BudgetFailure struct {
	Used      int64 `json:"used" description:"Actual plus reserved and uncertain amounts for this rejected limit."`
	Requested int64 `json:"requested" description:"Amount this rejected reservation attempted to consume."`
	Limit     int64 `json:"limit" description:"Explicit cumulative cap; zero means unset."`
	Actual    int64 `json:"actual" description:"Known amount charged to this rejected explicit limit; unavailable Host measurement uses HOST_USAGE_UNAVAILABLE instead."`
	Reserved  int64 `json:"reserved" description:"Amount reserved but not yet settled."`
	Uncertain int64 `json:"uncertain" description:"Conservative charged amount from unresolved work in this dimension."`
	Remaining int64 `json:"remaining" description:"Remaining capacity for this rejected explicit limit, clamped to zero."`
}
type Error struct {
	Code           string              `json:"code" description:"稳定的错误或缺口类别；用此值分派处理，不解析message措辞。"`
	Message        string              `json:"message" description:"可直接阅读的具体原因，不含敏感原始参数或内部堆栈。"`
	Details        []ContractViolation `json:"details,omitempty" description:"Concrete field/relationship violations, each with path, rule and message."`
	Budget         *BudgetFailure      `json:"budget,omitempty" description:"Numeric details for an explicit exceeded limit; unavailable Host usage has a separate error."`
	Gaps           []Gap               `json:"gaps,omitempty" description:"当前未满足条件的具体条目；与finish错误使用同一次计算。"`
	Current        *CurrentState       `json:"current,omitempty" description:"错误中返回的实际当前身份/摘要/变化路径；重新核对后使用，不盲目覆盖。"`
	ExistingResult *ResultReference    `json:"existing_result,omitempty" description:"已保存且可读取的原执行/记录身份；复用它，不重新执行。"`
	Outcome        string              `json:"-"`
	Execution      string              `json:"-"`
}

type CurrentState struct {
	TaskID              string   `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
	Revision            uint64   `json:"revision" description:"当前持久化修订。检查预留/结算都可能推进，不能自己加一。"`
	AuthorizationDigest Digest   `json:"authorization_digest,omitempty" description:"当前授权摘要。"`
	ContentDigest       Digest   `json:"content_digest,omitempty" description:"当前内容摘要。"`
	ChangedPaths        []string `json:"changed_paths,omitempty" description:"当前Task观察到的完整限定变化路径集合，不是权限名单。"`
}
type ResultReference struct {
	RunID      string `json:"run_id" description:"实际执行身份，来自原生check run；不重新构造执行来找结果。"`
	EvidenceID string `json:"evidence_id,omitempty" description:"该检查摘要采用的实际记录身份；null为没有结果。"`
	CheckID    string `json:"check_id,omitempty" description:"当前已保存检查的稳定身份。"`
}

func (e *Error) Error() string { return e.Message }
func Fail(code, p, message string) error {
	e := &Error{Code: code, Message: message}
	if p != "" {
		e.Details = []ContractViolation{{p, "requirement", message}}
	}
	return e
}
func Invalid(p, m string) error { return Fail("INVALID_ARGUMENT", p, m) }

var ErrInvalidArgument = Invalid("arguments", "invalid current contract")

func WithExplanation(_ error, m string) error { return Invalid("arguments", m) }
func validateID(v ID) error {
	if !v.IsValid() {
		return ErrInvalidArgument
	}
	return nil
}
func validateDigest(v Digest) error {
	if !v.IsValid() {
		return ErrInvalidArgument
	}
	return nil
}
func validateObjectID(v string) error {
	if len(v) != 40 && len(v) != 64 {
		return ErrInvalidArgument
	}
	_, e := hex.DecodeString(v)
	if e != nil || strings.ToLower(v) != v {
		return ErrInvalidArgument
	}
	return nil
}
func validateUTC(v time.Time) error {
	_, n := v.Zone()
	if v.IsZero() || n != 0 {
		return ErrInvalidArgument
	}
	return nil
}
func validateCanonicalPath(v string) error {
	if !filepath.IsAbs(v) || filepath.Clean(v) != v || len(v) > 4096 {
		return ErrInvalidArgument
	}
	return nil
}
func requireNormalizedText(v string, n int, required bool) error {
	if !utf8.ValidString(v) || strings.TrimSpace(v) != v || len(v) > n || required && v == "" {
		return ErrInvalidArgument
	}
	return nil
}
func cloneStringPointer(v *string) *string {
	if v == nil {
		return nil
	}
	r := *v
	return &r
}
func validateRepositoryRelativePath(v string) error {
	if v == "" || path.Clean(v) != v || path.IsAbs(v) || v == ".." || strings.HasPrefix(v, "../") || strings.ContainsAny(v, "\\:\x00") || strings.HasPrefix(v, ".git/") || v == ".git" {
		return ErrInvalidArgument
	}
	return nil
}
func ValidatePath(v string) error { return validateRepositoryRelativePath(v) }
func Text(p, v string) error {
	if requireNormalizedText(v, 8192, true) != nil {
		return Invalid(p, "must be nonempty UTF-8 text of at most 8192 bytes without surrounding whitespace")
	}
	return nil
}
func IDs(p string, vs []string) error {
	seen := map[string]bool{}
	if len(vs) > 64 {
		return Invalid(p, "at most 64 members")
	}
	for i, v := range vs {
		if !ID(v).IsValid() || seen[v] {
			return Invalid(fmt.Sprintf("%s[%d]", p, i), "must be a unique canonical identifier")
		}
		seen[v] = true
	}
	return nil
}
