package codescan

import ts "github.com/odvcencio/gotreesitter"

func (p *rustFile) patternNames(n *ts.Node) []*ts.Node {
	if n == nil {
		return nil
	}
	switch p.kind(n) {
	case "identifier", "shorthand_field_identifier":
		return []*ts.Node{n}
	case "field_pattern":
		if pattern := p.field(n, "pattern"); pattern != nil {
			return p.patternNames(pattern)
		}
		if name := p.field(n, "name"); name != nil {
			return []*ts.Node{name}
		}
	case "tuple_pattern", "tuple_struct_pattern", "struct_pattern", "slice_pattern", "reference_pattern", "mut_pattern", "captured_pattern", "match_pattern", "or_pattern":
		var result []*ts.Node
		for _, child := range p.named(n) {
			if child == p.field(n, "type") {
				continue
			}
			result = append(result, p.patternNames(child)...)
		}
		return result
	}
	return nil
}
