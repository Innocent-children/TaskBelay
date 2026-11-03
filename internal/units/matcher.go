package units

import (
	"context"
	"sort"
)

type Match struct {
	Pattern    string
	Start, End int
}
type state struct {
	Next    map[byte]int
	Fail    int
	Outputs []int
}
type Matcher struct {
	nodes    []state
	patterns []string
}

func NewMatcher(ctx context.Context, patterns []string) (*Matcher, error) {
	m := &Matcher{nodes: []state{{Next: map[byte]int{}}}}
	patterns = append([]string(nil), patterns...)
	sort.Strings(patterns)
	for _, pattern := range patterns {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pattern == "" || (len(m.patterns) > 0 && pattern == m.patterns[len(m.patterns)-1]) {
			continue
		}
		index := len(m.patterns)
		m.patterns = append(m.patterns, pattern)
		s := 0
		for i := range len(pattern) {
			c := pattern[i]
			n, ok := m.nodes[s].Next[c]
			if !ok {
				n = len(m.nodes)
				m.nodes = append(m.nodes, state{Next: map[byte]int{}})
				m.nodes[s].Next[c] = n
			}
			s = n
		}
		m.nodes[s].Outputs = append(m.nodes[s].Outputs, index)
	}
	queue := []int{0}
	for head := 0; head < len(queue); head++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s := queue[head]
		var keys []int
		for c := range m.nodes[s].Next {
			keys = append(keys, int(c))
		}
		sort.Ints(keys)
		for _, key := range keys {
			c := byte(key)
			n := m.nodes[s].Next[c]
			queue = append(queue, n)
			if s == 0 {
				continue
			}
			fail := m.nodes[s].Fail
			for fail != 0 {
				if _, ok := m.nodes[fail].Next[c]; ok {
					break
				}
				fail = m.nodes[fail].Fail
			}
			if target, ok := m.nodes[fail].Next[c]; ok {
				m.nodes[n].Fail = target
			}
			m.nodes[n].Outputs = append(m.nodes[n].Outputs, m.nodes[m.nodes[n].Fail].Outputs...)
		}
	}
	return m, nil
}

func (m *Matcher) Find(ctx context.Context, text string) ([]Match, error) {
	var out []Match
	s := 0
	for i := range len(text) {
		if i%PollBytes == 0 {
			if err := ctx.Err(); err != nil {
				return out, err
			}
		}
		c := text[i]
		for s != 0 {
			if _, ok := m.nodes[s].Next[c]; ok {
				break
			}
			s = m.nodes[s].Fail
		}
		if n, ok := m.nodes[s].Next[c]; ok {
			s = n
		}
		for _, idx := range m.nodes[s].Outputs {
			p := m.patterns[idx]
			out = append(out, Match{p, i + 1 - len(p), i + 1})
		}
	}
	return out, nil
}
