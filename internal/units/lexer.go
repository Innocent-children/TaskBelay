package units

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Source struct {
	Path, Blob, Mode string
	Data             []byte
}
type Syntax struct {
	Quotes, Comments, Keywords, Returns []string
	Samples                             int
	CallRatio                           float64
}
type Token struct {
	Kind, Text       string
	Start, End, Line int
}
type Unit struct {
	ID, Name                                     string
	Start, End, HeaderEnd, Line, EndLine, Parent int
	Tokens                                       []Token
}
type File struct {
	Path, Blob, Mode                 string
	Binary                           bool
	Tokens                           []Token
	Units                            []Unit
	Lines                            []string
	Layout                           string
	Consistency, Confidence, Natural float64
}

func Hash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s Syntax) Key() string { b, _ := json.Marshal(s); return Hash(string(b)) }

func Words(text string) []string {
	var words []string
	r := []rune(text)
	start := -1
	flush := func(end int) {
		if start >= 0 && end > start {
			words = append(words, strings.ToLower(string(r[start:end])))
		}
		start = -1
	}
	for i, c := range r {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		if unicode.IsUpper(c) && (unicode.IsLower(r[i-1]) || (i+1 < len(r) && unicode.IsLower(r[i+1]) && unicode.IsUpper(r[i-1]))) {
			flush(i)
			start = i
		}
	}
	flush(len(r))
	return words
}

func Analyze(ctx context.Context, source Source, syntax Syntax) (File, error) {
	f := File{Path: source.Path, Blob: source.Blob, Mode: source.Mode}
	if err := ctx.Err(); err != nil {
		return f, err
	}
	text := string(source.Data)
	if strings.IndexByte(text, 0) >= 0 || !utf8.Valid(source.Data) {
		f.Binary = true
		return f, nil
	}
	f.Lines = strings.SplitAfter(text, "\n")
	if len(f.Lines) > 0 && f.Lines[len(f.Lines)-1] == "" {
		f.Lines = f.Lines[:len(f.Lines)-1]
	}
	var err error
	f.Tokens, err = lex(ctx, text, syntax)
	if err != nil {
		return f, err
	}
	f.Units, f.Layout, f.Consistency, err = layout(ctx, f)
	if err != nil {
		return f, err
	}
	var prose, total int
	for _, t := range f.Tokens {
		if t.Kind == "space" {
			continue
		}
		total++
		if t.Kind == "comment" || t.Kind == "string" {
			prose += len(Words(t.Text))
		}
	}
	if total > 0 {
		f.Natural = float64(prose) / float64(total+prose)
	}
	return Reclassify(ctx, f, syntax)
}

func lex(ctx context.Context, text string, syntax Syntax) ([]Token, error) {
	var tokens []Token
	comments := append([]string(nil), syntax.Comments...)
	sort.Slice(comments, func(i, j int) bool {
		if len(comments[i]) == len(comments[j]) {
			return comments[i] < comments[j]
		}
		return len(comments[i]) > len(comments[j])
	})
	line, nextPoll := 1, 0
	for i := 0; i < len(text); {
		if i >= nextPoll {
			if err := ctx.Err(); err != nil {
				return tokens, err
			}
			nextPoll = i + PollBytes
		}
		start, at := i, line
		kind := "symbol"
		r, n := utf8.DecodeRuneInString(text[i:])
		matched := false
		for _, prefix := range comments {
			if prefix != "" && strings.HasPrefix(text[i:], prefix) {
				if end := strings.IndexByte(text[i:], '\n'); end >= 0 {
					i += end
				} else {
					i = len(text)
				}
				kind = "comment"
				matched = true
				break
			}
		}
		if !matched {
			for _, quote := range syntax.Quotes {
				if quote != "" && strings.HasPrefix(text[i:], quote) {
					i += len(quote)
					for i < len(text) {
						if text[i] == '\\' && i+1 < len(text) {
							_, size := utf8.DecodeRuneInString(text[i+1:])
							i += 1 + size
							continue
						}
						if strings.HasPrefix(text[i:], quote) {
							i += len(quote)
							break
						}
						_, size := utf8.DecodeRuneInString(text[i:])
						i += size
					}
					kind = "string"
					inner := strings.TrimSuffix(strings.TrimPrefix(text[start:i], quote), quote)
					if isKey(inner) {
						kind = "key"
					}
					matched = true
					break
				}
			}
		}
		if !matched {
			i += n
			switch {
			case unicode.IsSpace(r):
				kind = "space"
				for i < len(text) {
					v, size := utf8.DecodeRuneInString(text[i:])
					if !unicode.IsSpace(v) {
						break
					}
					i += size
				}
			case unicode.IsLetter(r) || r == '_':
				kind = "identifier"
				for i < len(text) {
					v, size := utf8.DecodeRuneInString(text[i:])
					if !unicode.IsLetter(v) && !unicode.IsDigit(v) && v != '_' {
						break
					}
					i += size
				}
			case unicode.IsDigit(r):
				kind = "number"
				for i < len(text) {
					v, size := utf8.DecodeRuneInString(text[i:])
					if !unicode.IsDigit(v) && v != '.' {
						break
					}
					i += size
				}
			default:
				if i < len(text) {
					pair := text[start : i+1]
					switch pair {
					case "<=", ">=", "==", "!=", "&&", "||", ":=", "=>", "->", "++", "--", "**":
						i++
					}
				}
			}
		}
		line += strings.Count(text[start:i], "\n")
		tokens = append(tokens, Token{Kind: kind, Text: text[start:i], Start: start, End: i, Line: at})
	}
	return tokens, ctx.Err()
}

// LiteralArgument 沿用文件的词法约定，判断一个实参是否为单个字面量（含负数）。
func LiteralArgument(ctx context.Context, text string, syntax Syntax) (bool, error) {
	if strings.IndexByte(text, 0) >= 0 || !utf8.ValidString(text) {
		return false, ctx.Err()
	}
	tokens, err := lex(ctx, text, syntax)
	tokens = Significant(tokens)
	if len(tokens) == 2 && tokens[0].Text == "-" && tokens[1].Kind == "number" {
		tokens = tokens[1:]
	}
	return len(tokens) == 1 && (tokens[0].Kind == "number" || tokens[0].Kind == "string" || tokens[0].Kind == "key"), err
}

// Reclassify 在词法约定不变时复用词流与版面，只更新名字和结构置信度。
func Reclassify(ctx context.Context, f File, syntax Syntax) (File, error) {
	f.Units = append([]Unit(nil), f.Units...)
	// 单元编号只依赖原词流、父级与同层次序，重新学习关键字不会改变编号。
	buildIDs := false
	for _, u := range f.Units {
		if u.ID == "" {
			buildIDs = true
			break
		}
	}
	if err := assignNames(ctx, &f, syntax, buildIDs); err != nil {
		return f, err
	}
	sample := float64(syntax.Samples) / (float64(syntax.Samples) + ConfidencePrior)
	f.Confidence = f.Consistency * sample * (1 + syntax.CallRatio) / 2
	return f, ctx.Err()
}

func isKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

// Significant 返回只读词流。已经滤过的词流可直接共享原切片。
func Significant(tokens []Token) []Token {
	if tokens == nil {
		return []Token{}
	}
	for i, t := range tokens {
		if t.Kind == "space" || t.Kind == "comment" {
			out := make([]Token, i, len(tokens)-1)
			copy(out, tokens[:i])
			for _, t := range tokens[i+1:] {
				if t.Kind != "space" && t.Kind != "comment" {
					out = append(out, t)
				}
			}
			return out
		}
	}
	return tokens[:len(tokens):len(tokens)]
}
func Sequence(tokens []Token, normalize bool) []string {
	var out []string
	for _, t := range tokens {
		if t.Kind == "space" || t.Kind == "comment" {
			continue
		}
		if out == nil {
			out = make([]string, 0, len(tokens))
		}
		if normalize && (t.Kind == "number" || t.Kind == "key" || t.Kind == "string") {
			out = append(out, "@"+t.Kind)
		} else {
			out = append(out, t.Text)
		}
	}
	return out
}
func Literal(t Token) string {
	if t.Kind == "string" || t.Kind == "key" {
		r := []rune(t.Text)
		if len(r) > 1 {
			return string(r[1 : len(r)-1])
		}
	}
	return t.Text
}
