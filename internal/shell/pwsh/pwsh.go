package pwsh

import (
	"sort"
	"strings"
	"unicode"

	"github.com/Innocent-children/taskbelay/internal/shell"
)

type Parser struct{}
type token struct {
	text     string
	from, to int
	literal  bool
}

/**
 * 只拆分文档规定的字面命令；变量、脚本块、子表达式和管道不作完整执行推断。
 * 不调用 PowerShell 来求值，因此解析本身不能执行项目命令。
 */
func (Parser) Parse(text, dir string) shell.Script {
	result := shell.Script{Display: text}
	environment := map[string]string{}
	known := true
	var tokens []token
	var masks []token
	sideEffect := false
	flush := func() {
		if len(tokens) == 0 {
			return
		}
		segment := shell.Segment{Dir: dir, Environment: shell.CopyEnvironment(environment), Known: known, SideEffect: sideEffect}
		for _, word := range tokens {
			segment.Args = append(segment.Args, word.text)
			segment.Known = segment.Known && word.literal
		}
		if len(segment.Args) > 0 && strings.HasPrefix(strings.ToLower(segment.Args[0]), "$env:") {
			key, value, ok := strings.Cut(segment.Args[0][5:], "=")
			if !ok && len(segment.Args) == 3 && segment.Args[1] == "=" {
				value, ok = segment.Args[2], true
			} else if ok && value == "" && len(segment.Args) == 2 {
				value = segment.Args[1]
			} else if len(segment.Args) != 1 {
				ok = false
			}
			_, _, validName := shell.Assignment(key + "=")
			if ok && validName {
				environment[key] = value
				segment.Kind, segment.Args = shell.Context, nil
				known = known && segment.Known
				masks = append(masks, token{from: tokens[0].from, to: tokens[len(tokens)-1].to, text: "$env:" + key + "='<redacted>'"})
			} else {
				segment.Known, known = false, false
				segment.Args = []string{"[未解析环境赋值]"}
				masks = append(masks, token{from: tokens[0].from, to: tokens[len(tokens)-1].to, text: "[未解析环境赋值]"})
			}
		}
		if len(segment.Args) > 0 {
			name := strings.ToLower(segment.Args[0])
			aliases := map[string]string{"pwd": "Get-Location", "gc": "Get-Content", "cat": "Get-Content", "type": "Get-Content", "ls": "Get-ChildItem", "dir": "Get-ChildItem", "gci": "Get-ChildItem", "gi": "Get-Item", "cd": "Set-Location", "chdir": "Set-Location", "sl": "Set-Location"}
			if target := aliases[name]; target != "" {
				segment.Args[0] = target
			}
			if strings.EqualFold(segment.Args[0], "Set-Location") {
				args := segment.Args[1:]
				if len(args) == 2 && (strings.EqualFold(args[0], "-Path") || strings.EqualFold(args[0], "-LiteralPath")) {
					args = args[1:]
				}
				if len(args) == 1 && segment.Known {
					dir = shell.Directory(dir, args[0])
					segment.Kind = shell.Context
					result.Complex = true
				} else {
					segment.Known, known = false, false
				}
			}
		}
		if !segment.Known && len(segment.Args) > 0 {
			keep := 0
			if tokens[0].literal {
				keep = 1
			}
			if len(tokens) > 1 && tokens[1].literal && len(segment.Args) > 1 {
				kind, _ := shell.Classify(shell.Segment{Args: segment.Args[:2], Known: true})
				if kind == shell.Verification {
					keep = 2
				}
			}
			for i := keep; i < len(segment.Args); i++ {
				segment.Args[i] = "[dynamic]"
			}
		}
		result.Segments = append(result.Segments, segment)
		tokens, sideEffect = nil, false
	}
	for i := 0; i < len(text); {
		r := rune(text[i])
		if text[i] == '\n' || text[i] == ';' {
			flush()
			i++
			continue
		}
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if text[i] == '#' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		if text[i] == '&' && i+1 < len(text) && text[i+1] == '&' {
			flush()
			i += 2
			continue
		}
		if text[i] == '|' {
			flush()
			result.Complex = true
			known = false
			i++
			if i < len(text) && text[i] == '|' {
				i++
			}
			continue
		}
		if text[i] == '&' && len(tokens) == 0 {
			i++
			continue
		}
		if text[i] == '>' || text[i] == '<' {
			sideEffect = true
			for i < len(text) && !unicode.IsSpace(rune(text[i])) && text[i] != ';' && text[i] != '\n' {
				i++
			}
			continue
		}
		start := i
		var value strings.Builder
		literal, quote := true, byte(0)
		for i < len(text) {
			c := text[i]
			if quote == 0 && (unicode.IsSpace(rune(c)) || strings.ContainsRune(";|&<>", rune(c))) {
				break
			}
			if c == '`' && quote != '\'' {
				if i+1 == len(text) {
					literal = false
					i++
					break
				}
				i++
				escapes := map[byte]byte{'n': '\n', 'r': '\r', 't': '\t', '0': 0, 'a': '\a', 'b': '\b', 'f': '\f', 'v': '\v'}
				if text[i] == '\n' {
					i++
					continue
				}
				if escaped, ok := escapes[text[i]]; ok {
					value.WriteByte(escaped)
				} else {
					value.WriteByte(text[i])
				}
				i++
				continue
			}
			if c == '\'' || c == '"' {
				if quote == 0 {
					quote = c
					i++
					continue
				}
				if quote == c {
					if i+1 < len(text) && text[i+1] == c {
						value.WriteByte(c)
						i += 2
						continue
					}
					quote = 0
					i++
					continue
				}
			}
			envPrefix := c == '$' && i == start && len(tokens) == 0 && strings.HasPrefix(strings.ToLower(text[i:]), "$env:")
			if quote != '\'' && ((c == '$' && !envPrefix) || c == '(' || c == ')' || c == '{' || c == '}' || c == '@') {
				literal = false
			}
			value.WriteByte(c)
			i++
		}
		if i == start {
			i++
			literal = false
		}
		if quote != 0 {
			literal = false
			result.Complex = true
		}
		word := token{text: value.String(), from: start, to: i, literal: literal}
		if key, _, ok := shell.Assignment(word.text); ok {
			masks = append(masks, token{from: start, to: i, text: key + "='<redacted>'"})
		}
		tokens = append(tokens, word)
	}
	flush()
	// 后面的替换先做，前面的源码偏移保持有效。
	sort.Slice(masks, func(i, j int) bool { return masks[i].from < masks[j].from })
	last := len(text)
	for i := len(masks) - 1; i >= 0; i-- {
		mask := masks[i]
		if mask.from <= len(result.Display) && mask.to <= last {
			result.Display = result.Display[:mask.from] + mask.text + result.Display[mask.to:]
			last = mask.from
		}
	}
	for _, segment := range result.Segments {
		if !segment.Known {
			result.Display = "[含未解析结构的 PowerShell 命令]"
			break
		}
	}
	return result
}
