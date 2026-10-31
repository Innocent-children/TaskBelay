package codescan

import (
	"github.com/Innocent-children/taskbelay/internal/lang"
)

type Source struct {
	Path string
	Blob string
	Mode string
	Data []byte `json:"-"`
}

type Missing struct {
	Path      string
	Operation string
	Reason    string
	Kind      string
}

type Span struct {
	Start, End         uint32
	StartLine, EndLine int
}

type Token struct {
	Kind string
	Text string
	Span Span
}

type Declaration struct {
	Usage                                 *Usage
	Visibility                            Visibility
	Shape                                 *FunctionShape
	ID, File, Name, Owner, Kind, NodeType string
	Span, NameSpan                        Span
	Segments, NameSpans                   []Span
	Test                                  bool
	Text                                  string `json:"-"`
	Documentation                         string
	Tokens                                []Token
	References                            []string
	FullHash, CodeHash, ShapeHash         string
	LiteralFreeHash                       string
}

type Usage struct {
	References                                                       []Use
	TypeParameters                                                   []string
	Public, Abstract, Generic, Inferred, Error, Sentinel, Deprecated bool
	Alias                                                            string
	Implicit, Methods                                                []string
	Implementations, TypeArguments, ErrorChecks                      []Use
	Parameters, Fields                                               []Slot
	Unknown                                                          []Use
}

type Use struct {
	File, Declaration, Value, Reason string
	Span                             Span
	Test, Candidate                  bool
}

type Value struct {
	Kind, Text string
	Implicit   bool
	At         Use
}

type Slot struct {
	Name, Type                                                     string
	Span                                                           Span
	Read, Written, Function, Variadic, Interface, Public, External bool
	Values                                                         []Value
	Functions, Unknown                                             []Use
}

func (d Declaration) Location() string {
	name := d.Name
	if d.Owner != "" {
		name = d.Owner + "." + name
	}
	return d.File + ":" + name
}

func (d Declaration) Ranges() []Span {
	if len(d.Segments) > 0 {
		return d.Segments
	}
	return []Span{d.Span}
}

func (d Declaration) NameRanges() []Span {
	if len(d.NameSpans) > 0 {
		return d.NameSpans
	}
	return []Span{d.NameSpan}
}

type FunctionShape struct {
	Simple     bool
	SimpleKind string
	Parameters []string
	Branches   []Span
	Forwarder  string
}

type File struct {
	Source
	Package      string
	Kind         lang.FileKind
	CodeComplete bool
	Declarations []Declaration
	References   []string
	Literals     []string
	Dependencies []Dependency
	Missing      []Missing
	Entries      []Entry
	Risks        []Risk
	Tests        []TestCase
	Operations   map[string]Operation
}

type Visibility struct {
	Scope, Package, Reason, Kind string
}

type Assertion struct {
	Kind, Evidence string
	Span           Span
}

type TestCase struct {
	File, Name, Framework, Declaration string
	Span                               Span
	Parameters                         []Span
	Context                            []Span
	Assertions                         []Assertion
	References                         []string
	Unknown                            []string
	CodeHash                           string
}

func (test TestCase) Location() string { return "test:" + test.File + ":" + test.Name }

type Operation struct {
	Complete bool
	Reason   string
	Kind     string
}

type Entry struct {
	Kind, Name, Method, Declaration, Evidence string
	Span                                      Span
}

type Risk struct {
	Kind, Declaration, Evidence string
	Span                        Span
}

func (e Entry) Location() string {
	if e.Kind == "route" && e.Method != "" {
		return e.Kind + ":" + e.Method + " " + e.Name
	}
	return e.Kind + ":" + e.Name
}

type Snapshot struct {
	Tree    string
	Files   map[string]*File
	Missing []Missing
}

func (s Snapshot) Declarations() map[string]Declaration {
	result := map[string]Declaration{}
	for _, file := range s.Files {
		for _, declaration := range file.Declarations {
			result[declaration.ID] = declaration
		}
	}
	return result
}

func addUse(uses *[]Use, use Use) {
	for _, old := range *uses {
		if old.File == use.File && old.Span.Start == use.Span.Start && old.Span.End == use.Span.End && old.Value == use.Value && old.Reason == use.Reason {
			return
		}
	}
	*uses = append(*uses, use)
}
