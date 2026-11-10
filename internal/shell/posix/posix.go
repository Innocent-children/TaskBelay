package posix

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/shell"
	"mvdan.cc/sh/v3/syntax"
)

type Parser struct{}

type redaction struct {
	from, to int
	text     string
}
type state struct {
	dir         string
	environment map[string]string
	known       bool
}
type reader struct {
	text       string
	result     shell.Script
	redactions []redaction
}

func (Parser) Parse(text, dir string) shell.Script {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(text), "")
	if err != nil {
		return shell.Script{Display: "[无法解析的 POSIX 命令]", Complex: true, Missing: []string{"POSIX 语法未能完整解析"}}
	}
	r := reader{text: text, result: shell.Script{Display: text}}
	s := state{dir: dir, environment: map[string]string{}, known: true}
	r.statements(file.Stmts, &s, true)
	sort.Slice(r.redactions, func(i, j int) bool { return r.redactions[i].from > r.redactions[j].from })
	last := len(text)
	for _, redaction := range r.redactions {
		if redaction.from < 0 || redaction.to > last || redaction.from > redaction.to {
			continue
		}
		r.result.Display = r.result.Display[:redaction.from] + redaction.text + r.result.Display[redaction.to:]
		last = redaction.from
	}
	return r.result
}

// 每层语句列表都可能用分号或换行吞掉前一条命令的失败状态。
func (r *reader) statements(stmts []*syntax.Stmt, s *state, known bool) {
	r.result.Sequential = r.result.Sequential || len(stmts) > 1
	for _, stmt := range stmts {
		dir := s.dir
		r.statement(stmt, s, known)
		if s.dir != dir {
			s.known = false
		}
	}
}

func (r *reader) statement(stmt *syntax.Stmt, s *state, known bool) {
	if stmt == nil {
		return
	}
	known = known && !stmt.Negated && !stmt.Coprocess && !stmt.Disown
	sideEffect := writes(stmt.Redirs)
	if stmt.Background {
		r.result.Complex = true
	}
	switch command := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		op := command.Op.String()
		// 管道的退出状态只来自最后一段；前面各段是否只读要等分类后才知道。
		piped := op == "|" && len(stmt.Redirs) == 0
		if op != "&&" && !piped {
			r.result.Complex = true
		}
		left := *s
		left.environment = shell.CopyEnvironment(s.environment)
		count := len(r.result.Segments)
		r.statement(command.X, &left, known && (op == "&&" || piped))
		if piped {
			for i := count; i < len(r.result.Segments); i++ {
				r.result.Segments[i].Piped = true
			}
		}
		if op == "&&" {
			*s = left
		}
		r.statement(command.Y, s, known && (op == "&&" || piped))
	case *syntax.CallExpr:
		segment := shell.Segment{Dir: s.dir, Environment: shell.CopyEnvironment(s.environment), Known: known && s.known, SideEffect: sideEffect, Background: stmt.Background, Input: r.input(stmt.Redirs)}
		for _, assignment := range command.Assigns {
			if assignment.Name == nil || assignment.Array != nil || assignment.Append || assignment.Index != nil {
				segment.Known = false
				continue
			}
			value, ok := literal(assignment.Value)
			segment.Known = segment.Known && ok
			segment.Environment[assignment.Name.Value] = value
			if assignment.Value != nil {
				r.mask(assignment.Value, "'<redacted>'")
			}
		}
		for _, word := range command.Args {
			value, ok := literal(word)
			if !ok {
				value = "[dynamic]"
				segment.Known = false
				r.mask(word, "'[dynamic]'")
			}
			segment.Args = append(segment.Args, value)
			if key, _, ok := shell.Assignment(value); ok {
				r.mask(word, key+"='<redacted>'")
			}
		}
		if len(segment.Args) == 0 {
			segment.Kind = shell.Context
			s.environment, s.known = segment.Environment, segment.Known
		} else if segment.Args[0] == "cd" {
			args := segment.Args[1:]
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if len(args) == 1 && !strings.HasPrefix(args[0], "-") && segment.Known {
				s.dir = shell.Directory(s.dir, args[0])
				segment.Kind = shell.Context
			} else {
				segment.Known = false
				s.known = false
			}
		}
		r.result.Segments = append(r.result.Segments, segment)
	case *syntax.Subshell:
		local := *s
		local.environment = shell.CopyEnvironment(s.environment)
		r.statements(command.Stmts, &local, known && !sideEffect)
		if sideEffect {
			r.result.Complex = true
		}
	case *syntax.Block:
		r.statements(command.Stmts, s, known && !sideEffect)
	default:
		r.result.Complex = true
		r.result.Missing = append(r.result.Missing, "条件、循环、函数或其他 shell 结构未核对执行范围")
		r.result.Segments = append(r.result.Segments, shell.Segment{Dir: s.dir, Kind: shell.Other})
		r.mask(stmt, "[未解析结构]")
	}
}

/**
 * 只有写入文件的重定向算副作用。合并输出流、丢弃输出和读取输入不改变工作区。
 */
func writes(redirs []*syntax.Redirect) bool {
	for _, redir := range redirs {
		switch redir.Op {
		case syntax.RdrIn, syntax.DplIn, syntax.DplOut, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			continue
		}
		if target, ok := literal(redir.Word); ok && target == "/dev/null" {
			continue
		}
		return true
	}
	return false
}

/**
 * input 返回从标准输入交给程序的内容摘要。here-document 里的脚本和输入文件名
 * 决定程序实际做什么，参数相同而输入不同的两次调用不是同一条命令。
 */
func (r *reader) input(redirs []*syntax.Redirect) string {
	var parts []string
	for _, redir := range redirs {
		var node syntax.Node
		switch redir.Op {
		case syntax.Hdoc, syntax.DashHdoc:
			if redir.Hdoc != nil {
				node = redir.Hdoc
			}
		case syntax.RdrIn, syntax.WordHdoc:
			if redir.Word != nil {
				node = redir.Word
			}
		}
		if node == nil {
			continue
		}
		from, to := int(node.Pos().Offset()), int(node.End().Offset())
		if from < 0 || to > len(r.text) || from > to {
			continue
		}
		parts = append(parts, redir.Op.String(), r.text[from:to])
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

func (r *reader) mask(node syntax.Node, text string) {
	r.redactions = append(r.redactions, redaction{int(node.Pos().Offset()), int(node.End().Offset()), text})
}

func literal(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", true
	}
	return parts(word.Parts, false)
}

func parts(items []syntax.WordPart, quoted bool) (string, bool) {
	var result strings.Builder
	for _, item := range items {
		switch part := item.(type) {
		case *syntax.Lit:
			text := part.Value
			for i := 0; i < len(text); i++ {
				if text[i] == '\\' && i+1 < len(text) {
					next := text[i+1]
					if !quoted || strings.ContainsRune("$`\"\\\n", rune(next)) {
						i++
						if next != '\n' {
							result.WriteByte(next)
						}
						continue
					}
				}
				if !quoted && strings.ContainsRune("*?[~", rune(text[i])) {
					return "", false
				}
				result.WriteByte(text[i])
			}
		case *syntax.SglQuoted:
			if part.Dollar {
				return "", false
			}
			result.WriteString(part.Value)
		case *syntax.DblQuoted:
			if part.Dollar {
				return "", false
			}
			value, ok := parts(part.Parts, true)
			if !ok {
				return "", false
			}
			result.WriteString(value)
		default:
			return "", false
		}
	}
	return result.String(), true
}
