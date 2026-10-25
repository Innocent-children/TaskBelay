package codescan

import (
	"slices"

	"github.com/Innocent-children/taskbelay/internal/lang/java"
)

func javaKnownMethod(target string) bool {
	if len(javaRiskKinds(target)) > 0 || java.Rules.ReceiverMethods[target] != "" || java.TestAssertion(target) != "" {
		return true
	}
	for _, rule := range java.Rules.Entries {
		if rule.Target == target {
			return true
		}
	}
	return false
}
func (a *javaAnalysis) parentTypes(owner string, depth int) []string {
	if depth > 40 || a.ctx.Err() != nil || len(a.types[owner]) != 1 {
		return nil
	}
	item := a.types[owner][0]
	p := item.file
	var parents []string
	for _, child := range p.named(item.node) {
		if !slices.Contains([]string{"superclass", "super_interfaces", "extends_interfaces"}, p.kind(child)) {
			continue
		}
		for _, typ := range p.named(child) {
			if p.kind(typ) == "type_list" {
				for _, element := range p.named(typ) {
					parents = append(parents, p.typeName(element, depth+1))
				}
			} else {
				parents = append(parents, p.typeName(typ, depth+1))
			}
		}
	}
	return parents
}
func (a *javaAnalysis) apiMethod(owner, name string, depth int) string {
	if owner == "" || depth > 40 || a.ctx.Err() != nil {
		return ""
	}
	target := owner + "." + name
	if len(a.methods[target]) > 0 || javaKnownMethod(target) {
		return target
	}
	var matches []string
	for _, parent := range a.parentTypes(owner, depth+1) {
		if parent == "" {
			return ""
		}
		candidate := a.apiMethod(parent, name, depth+1)
		if (len(a.methods[candidate]) > 0 || javaKnownMethod(candidate)) && !slices.Contains(matches, candidate) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	if len(matches) > 1 {
		return ""
	}
	return target
}
func (a *javaAnalysis) memberBindings(owner, name string, depth int) []*javaBinding {
	if owner == "" || depth > 40 || a.ctx.Err() != nil {
		return nil
	}
	if bindings := a.members[owner+"."+name]; len(bindings) > 0 {
		return bindings
	}
	var result []*javaBinding
	for _, parent := range a.parentTypes(owner, depth+1) {
		for _, binding := range a.memberBindings(parent, name, depth+1) {
			if !slices.Contains(result, binding) {
				result = append(result, binding)
			}
		}
	}
	return result
}
