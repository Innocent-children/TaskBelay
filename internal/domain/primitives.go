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
	Path    string        `json:"path"`
	Rule    ViolationRule `json:"rule"`
	Message string        `json:"message"`
}

func ExplainedViolation(p string, r ViolationRule, m string) ContractViolation {
	return ContractViolation{p, r, m}
}

type BudgetFailure struct {
	Used      int64 `json:"used"`
	Requested int64 `json:"requested"`
	Limit     int64 `json:"limit"`
	Actual    int64 `json:"actual"`
	Reserved  int64 `json:"reserved"`
	Uncertain int64 `json:"uncertain"`
	Remaining int64 `json:"remaining"`
}
type Error struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Details []ContractViolation `json:"details,omitempty"`
	Budget  *BudgetFailure      `json:"budget,omitempty"`
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
