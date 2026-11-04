package units

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Match struct {
	Pattern    string
	Start, End int
}

// WholeWord 避免把标识符内部的短子串当成独立种子或用例名。
func WholeWord(text string, m Match) bool {
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	first, _ := utf8.DecodeRuneInString(m.Pattern)
	last, _ := utf8.DecodeLastRuneInString(m.Pattern)
	if m.Start > 0 && word(first) {
		r, _ := utf8.DecodeLastRuneInString(text[:m.Start])
		if word(r) {
			return false
		}
	}
	if m.End < len(text) && word(last) {
		r, _ := utf8.DecodeRuneInString(text[m.End:])
		if word(r) {
			return false
		}
	}
	return true
}

func HasWord(text, word string) bool {
	if word == "" {
		return false
	}
	for offset := 0; offset < len(text); {
		i := strings.Index(text[offset:], word)
		if i < 0 {
			return false
		}
		i += offset
		if WholeWord(text, Match{word, i, i + len(word)}) {
			return true
		}
		offset = i + len(word)
	}
	return false
}

type MatcherEdge struct {
	Byte byte
	Next int
}
type MatcherState struct {
	Edges   []MatcherEdge
	Fail    int
	Outputs []string
}

func (s MatcherState) next(c byte) (int, bool) {
	for _, edge := range s.Edges {
		if edge.Byte == c {
			return edge.Next, true
		}
	}
	return 0, false
}

type Matcher struct {
	nodes    []MatcherState
	patterns []string
}

func NewMatcher(ctx context.Context, patterns []string) (*Matcher, error) {
	m := &Matcher{nodes: []MatcherState{{}}}
	patterns = append([]string(nil), patterns...)
	sort.Strings(patterns)
	for _, pattern := range patterns {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pattern == "" || (len(m.patterns) > 0 && pattern == m.patterns[len(m.patterns)-1]) {
			continue
		}
		m.patterns = append(m.patterns, pattern)
		s := 0
		for i := range len(pattern) {
			c := pattern[i]
			n, ok := m.nodes[s].next(c)
			if !ok {
				n = len(m.nodes)
				m.nodes = append(m.nodes, MatcherState{})
				m.nodes[s].Edges = append(m.nodes[s].Edges, MatcherEdge{c, n})
			}
			s = n
		}
		m.nodes[s].Outputs = append(m.nodes[s].Outputs, pattern)
	}
	queue := []int{0}
	for head := 0; head < len(queue); head++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s := queue[head]
		edges := m.nodes[s].Edges
		sort.Slice(edges, func(i, j int) bool { return edges[i].Byte < edges[j].Byte })
		for _, edge := range edges {
			c, n := edge.Byte, edge.Next
			queue = append(queue, n)
			if s == 0 {
				continue
			}
			fail := m.nodes[s].Fail
			for fail != 0 {
				if _, ok := m.nodes[fail].next(c); ok {
					break
				}
				fail = m.nodes[fail].Fail
			}
			if target, ok := m.nodes[fail].next(c); ok {
				m.nodes[n].Fail = target
			}
			m.nodes[n].Outputs = append(m.nodes[n].Outputs, m.nodes[m.nodes[n].Fail].Outputs...)
		}
	}
	return m, nil
}

func (m *Matcher) Find(ctx context.Context, text string) ([]Match, error) {
	return FindStates(ctx, text, func(i int) (MatcherState, error) { return m.nodes[i], nil })
}

func (m *Matcher) States() []MatcherState { return m.nodes }

/**
 * FindStates 按文本触及的状态读取自动机，状态可来自内存或调用方的索引。
 */
func FindStates(ctx context.Context, text string, read func(int) (MatcherState, error)) ([]Match, error) {
	var out []Match
	s := 0
	for i := range len(text) {
		if i%PollBytes == 0 {
			if err := ctx.Err(); err != nil {
				return out, err
			}
		}
		c := text[i]
		state, err := read(s)
		if err != nil {
			return out, err
		}
		for s != 0 {
			if _, ok := state.next(c); ok {
				break
			}
			s = state.Fail
			state, err = read(s)
			if err != nil {
				return out, err
			}
		}
		if n, ok := state.next(c); ok {
			s = n
			state, err = read(s)
			if err != nil {
				return out, err
			}
		}
		for _, p := range state.Outputs {
			out = append(out, Match{p, i + 1 - len(p), i + 1})
		}
	}
	return out, nil
}
