package shell

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
)

type Kind string

const (
	Verification Kind = "verification"
	ReadOnly     Kind = "read_only"
	Other        Kind = "other"
	Context      Kind = "context"
)

type Segment struct {
	Args             []string          `json:"args"`
	Dir              string            `json:"directory"`
	Shell            event.Shell       `json:"shell"`
	Prefix           []string          `json:"prefix,omitempty"`
	Environment      map[string]string `json:"-"`
	Unset            []string          `json:"-"`
	ClearEnvironment bool              `json:"clear_environment,omitempty"`
	Known            bool              `json:"known"`
	SideEffect       bool              `json:"side_effect"`
	Background       bool              `json:"background"`
	Kind             Kind              `json:"kind"`
	Language         string            `json:"language,omitempty"`
	EnvironmentID    string            `json:"environment_id"`
}

type Script struct {
	Display  string    `json:"display"`
	Segments []Segment `json:"segments"`
	Complex  bool      `json:"complex"`
	Missing  []string  `json:"missing,omitempty"`
}

type Parser interface{ Parse(text, dir string) Script }

func Base(command string) string {
	command = strings.ReplaceAll(command, "\\", "/")
	if index := strings.LastIndexByte(command, '/'); index >= 0 {
		command = command[index+1:]
	}
	return strings.TrimSuffix(strings.ToLower(command), ".exe")
}

func Directory(base, target string) string {
	// Windows 盘符在 Unix 的构造事件中也保持绝对语义。
	if len(target) > 2 && target[1] == ':' && (target[2] == '\\' || target[2] == '/') {
		return strings.ReplaceAll(target, "\\", "/")
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(base, target)
}

func Assignment(word string) (string, string, bool) {
	name, value, found := strings.Cut(word, "=")
	if !found || name == "" {
		return "", "", false
	}
	for i, r := range name {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (i == 0 || r < '0' || r > '9') {
			return "", "", false
		}
	}
	return name, value, true
}

func CopyEnvironment(source map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range source {
		result[key] = value
	}
	return result
}

func Parse(text, dir string, style event.Shell, parsers map[event.Shell]Parser) Script {
	return parse(text, dir, style, parsers, 0)
}

func parse(text, dir string, style event.Shell, parsers map[event.Shell]Parser, depth int) Script {
	p := parsers[style]
	if p == nil || depth > 5 {
		return Script{Display: "[未解析命令]", Complex: true, Missing: []string{"shell 写法或包装层数无法核对"}}
	}
	result := p.Parse(text, dir)
	var segments []Segment
	for _, segment := range result.Segments {
		segment.Shell = style
		unwrap(&segment)
		inner, innerStyle, ok := wrapped(segment.Args)
		if ok {
			child := parse(inner, segment.Dir, innerStyle, parsers, depth+1)
			if child.Display != inner {
				if strings.Contains(result.Display, inner) {
					result.Display = strings.Replace(result.Display, inner, child.Display, 1)
				} else {
					result.Display = "[外层 " + Base(segment.Args[0]) + "] " + child.Display + "；其余命令见解析参数"
				}
			}
			result.Complex = result.Complex || child.Complex
			result.Missing = append(result.Missing, child.Missing...)
			for _, nested := range child.Segments {
				environment := CopyEnvironment(segment.Environment)
				if nested.ClearEnvironment {
					environment = map[string]string{}
				}
				for key, value := range nested.Environment {
					environment[key] = value
				}
				nested.Environment = environment
				nested.ClearEnvironment = nested.ClearEnvironment || segment.ClearEnvironment
				nested.Unset = append(append([]string(nil), segment.Unset...), nested.Unset...)
				nested.Prefix = append(append(append([]string(nil), segment.Prefix...), segment.Args[:len(segment.Args)-1]...), nested.Prefix...)
				nested.Known = nested.Known && segment.Known
				nested.SideEffect = nested.SideEffect || segment.SideEffect
				nested.Background = nested.Background || segment.Background
				segments = append(segments, nested)
			}
			continue
		}
		segment.Kind, segment.Language = Classify(segment)
		segments = append(segments, segment)
	}
	result.Segments = segments
	return result
}

func wrapped(args []string) (string, event.Shell, bool) {
	if len(args) < 3 {
		return "", "", false
	}
	name := Base(args[0])
	if name == "sh" || name == "bash" || name == "zsh" {
		for i := 1; i < len(args)-1; i++ {
			if strings.HasPrefix(args[i], "-") && strings.Contains(strings.TrimLeft(args[i], "-"), "c") && i+2 == len(args) {
				return args[i+1], event.POSIX, true
			}
		}
	}
	if name == "pwsh" || name == "powershell" {
		for i := 1; i < len(args)-1; i++ {
			if (strings.EqualFold(args[i], "-Command") || strings.EqualFold(args[i], "-c")) && i+2 == len(args) {
				return args[i+1], event.PowerShell, true
			}
		}
	}
	return "", "", false
}

func unwrap(s *Segment) {
	if s.Environment == nil {
		s.Environment = map[string]string{}
	}
	for len(s.Args) > 0 {
		name := Base(s.Args[0])
		n := 0
		switch name {
		case "env":
			n = 1
			for n < len(s.Args) {
				arg := s.Args[n]
				if arg == "--" {
					n++
					break
				}
				if arg == "-i" || arg == "--ignore-environment" {
					s.ClearEnvironment = true
					n++
					continue
				}
				if (arg == "-u" || arg == "--unset") && n+1 < len(s.Args) {
					s.Unset = append(s.Unset, s.Args[n+1])
					n += 2
					continue
				}
				if key, value, ok := Assignment(arg); ok {
					s.Environment[key] = value
					n++
					continue
				}
				if strings.HasPrefix(arg, "-") {
					s.Known = false
				}
				break
			}
		case "npx", "bunx":
			n = 1
			for n < len(s.Args) && (s.Args[n] == "--no-install" || s.Args[n] == "--yes" || s.Args[n] == "-y") {
				n++
			}
		case "pnpm", "bundle":
			if len(s.Args) > 1 && s.Args[1] == "exec" {
				n = 2
			}
		case "uv", "poetry", "pipenv":
			if len(s.Args) > 1 && s.Args[1] == "run" {
				n = 2
			}
		case "python", "python3", "python3.12", "python3.13", "python3.14":
			if len(s.Args) > 2 && s.Args[1] == "-m" {
				n = 2
			}
		}
		if n == 0 || n >= len(s.Args) {
			break
		}
		// 环境赋值仅进入摘要，不把凭据带入保存的前缀或参数。
		for _, part := range s.Args[:n] {
			if key, _, ok := Assignment(part); ok {
				part = key + "=<redacted>"
			}
			s.Prefix = append(s.Prefix, part)
		}
		s.Args = s.Args[n:]
	}
}

func (s *Script) HashEnvironment(inherited []string) string {
	var ids []string
	for i := range s.Segments {
		segment := &s.Segments[i]
		values := map[string]string{}
		if !segment.ClearEnvironment {
			for _, item := range inherited {
				key, value, ok := strings.Cut(item, "=")
				if ok {
					values[key] = value
				}
			}
		}
		for _, key := range segment.Unset {
			delete(values, key)
		}
		for key, value := range segment.Environment {
			values[key] = value
		}
		data, _ := json.Marshal(values)
		segment.EnvironmentID = fmt.Sprintf("%x", sha256.Sum256(data))
		ids = append(ids, segment.EnvironmentID)
	}
	data, _ := json.Marshal(ids)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (s Script) HasVerification() bool {
	for _, segment := range s.Segments {
		if segment.Kind == Verification {
			return true
		}
	}
	return false
}

func (s Script) Mixed() bool {
	if s.Complex {
		return true
	}
	for _, segment := range s.Segments {
		if segment.Kind == Context && (!segment.Known || segment.SideEffect || segment.Background) {
			return true
		}
		if segment.Kind != Verification && segment.Kind != Context {
			return true
		}
	}
	return false
}

func (s Script) CommandKey() string {
	var parts []Segment
	for _, segment := range s.Segments {
		if segment.Kind == Context {
			continue
		}
		segment.EnvironmentID = ""
		parts = append(parts, segment)
	}
	data, _ := json.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
