package posix

import (
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
	for _, stmt := range file.Stmts {
		before := s.dir
		r.statement(stmt, &s, true)
		if s.dir != before {
			s.known = false
		}
	}
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

func (r *reader) statement(stmt *syntax.Stmt, s *state, known bool) {
	if stmt == nil {
		return
	}
	known = known && !stmt.Negated && !stmt.Coprocess && !stmt.Disown
	sideEffect := len(stmt.Redirs) > 0
	if stmt.Background {
		r.result.Complex = true
	}
	switch command := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		op := command.Op.String()
		if op != "&&" {
			r.result.Complex = true
		}
		left := *s
		left.environment = shell.CopyEnvironment(s.environment)
		r.statement(command.X, &left, known && op == "&&")
		if op == "&&" {
			*s = left
		}
		r.statement(command.Y, s, known && op == "&&")
	case *syntax.CallExpr:
		segment := shell.Segment{Dir: s.dir, Environment: shell.CopyEnvironment(s.environment), Known: known && s.known, SideEffect: sideEffect, Background: stmt.Background}
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
		for _, child := range command.Stmts {
			r.statement(child, &local, known && !sideEffect)
		}
		if sideEffect {
			r.result.Complex = true
		}
	case *syntax.Block:
		for _, child := range command.Stmts {
			r.statement(child, s, known && !sideEffect)
		}
	default:
		r.result.Complex = true
		r.result.Missing = append(r.result.Missing, "条件、循环、函数或其他 shell 结构未核对执行范围")
		r.result.Segments = append(r.result.Segments, shell.Segment{Dir: s.dir, Kind: shell.Other})
		r.mask(stmt, "[未解析结构]")
	}
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
