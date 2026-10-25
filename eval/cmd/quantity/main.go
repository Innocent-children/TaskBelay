// Command quantity measures fixed candidate trees for evaluation only.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/lang"
)

type unknown struct {
	File   string `json:"file"`
	Metric string `json:"metric"`
	Reason string `json:"reason"`
}

type report struct {
	BaseTree          string            `json:"base_tree"`
	Tree              string            `json:"tree"`
	Scope             string            `json:"scope"`
	NonTestAddedLines *int              `json:"non_test_added_lines"`
	AddedTestCount    *int              `json:"added_test_count"`
	SourceLines       map[string]int    `json:"source_lines"`
	AddedTests        []string          `json:"added_tests"`
	TestMigrations    map[string]string `json:"test_migrations"`
	Unknown           []unknown         `json:"unknown"`
}

type comparison struct {
	Candidate    report   `json:"candidate"`
	Reference    report   `json:"reference"`
	CodeRatio    *float64 `json:"code_ratio"`
	TestRatio    *float64 `json:"test_ratio"`
	Adoption     string   `json:"adoption"`
	RuntimeCosts string   `json:"runtime_costs"`
}

func ratios(candidate, reference report) comparison {
	result := comparison{Candidate: candidate, Reference: reference, Adoption: "not_evaluated", RuntimeCosts: "not_evaluated"}
	if candidate.NonTestAddedLines != nil && reference.NonTestAddedLines != nil && *reference.NonTestAddedLines > 0 {
		value := float64(*candidate.NonTestAddedLines) / float64(*reference.NonTestAddedLines)
		result.CodeRatio = &value
	}
	if candidate.AddedTestCount != nil && reference.AddedTestCount != nil {
		value := float64(*candidate.AddedTestCount) / float64(max(1, *reference.AddedTestCount))
		result.TestRatio = &value
	}
	return result
}

func regular(mode string) bool { return mode == "100644" || mode == "100755" }

func supported(language string) bool {
	switch language {
	case "go", "python", "javascript", "typescript", "tsx", "java", "rust":
		return true
	}
	return false
}

func lineCount(data []byte) int {
	count := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}
	return count
}

/*
snapshot reads fixed objects only. It does not capture the working directory,
write an index, or create the empty blob used by some diff helpers.
*/
func snapshot(ctx context.Context, workspace gitstate.Workspace, tree string, cache map[string][]byte) (codescan.Snapshot, error) {
	files, err := workspace.TreeFiles(ctx, tree)
	if err != nil {
		return codescan.Snapshot{}, err
	}
	var sources []codescan.Source
	for name, file := range files {
		kind := lang.Classify(name, nil)
		if !kind.Source || !supported(kind.Language) {
			continue
		}
		data, found := cache[file.Blob]
		if !found {
			data, err = workspace.ReadBlob(ctx, file.Blob)
			if err != nil {
				return codescan.Snapshot{}, err
			}
			cache[file.Blob] = data
		}
		sources = append(sources, codescan.Source{Path: name, Blob: file.Blob, Mode: file.Mode, Data: data})
	}
	return codescan.Analyze(ctx, tree, sources), ctx.Err()
}

func testMap(snapshot codescan.Snapshot) map[string]codescan.TestCase {
	result := map[string]codescan.TestCase{}
	for _, file := range snapshot.Files {
		for _, test := range file.Tests {
			result[test.Location()] = test
		}
	}
	return result
}

/*
measure uses the same source and test classification for the candidate and
reference. Counts are unavailable when changed test bodies cannot be enumerated.
*/
func measure(ctx context.Context, repository, baseTree, tree string) (report, error) {
	result := report{BaseTree: baseTree, Tree: tree, Scope: "Go, Python, JavaScript/TypeScript, Java and Rust fixed-tree quantities; correctness and runtime cost are separate",
		SourceLines: map[string]int{}, AddedTests: []string{}, Unknown: []unknown{}}
	workspace, err := gitstate.Discover(ctx, repository)
	if err != nil {
		return result, err
	}
	changes, err := workspace.Diff(ctx, baseTree, tree)
	if err != nil {
		return result, err
	}
	cache := map[string][]byte{}
	before, err := snapshot(ctx, workspace, baseTree, cache)
	if err != nil {
		return result, err
	}
	after, err := snapshot(ctx, workspace, tree, cache)
	if err != nil {
		return result, err
	}
	declarations, err := codescan.Compare(ctx, before, after, nil)
	if err != nil {
		return result, err
	}
	result.TestMigrations = codescan.TestMigrations(before, after, declarations)
	oldTests, newTests := testMap(before), testMap(after)
	oldFacts, newFacts := fileQuantities(before), fileQuantities(after)
	migrated := map[string]bool{}
	for _, target := range result.TestMigrations {
		migrated[target] = true
	}
	changedPaths := map[string]bool{}
	sourceCount, sourceComplete, testsComplete := 0, true, true
	for _, change := range changes {
		changedPaths[change.Path], changedPaths[change.BeforePath] = true, true
		kind := lang.Classify(change.Path, nil)
		if kind.Source && !supported(kind.Language) && change.After.Blob != "" {
			result.Unknown = append(result.Unknown, unknown{change.Path, "source_and_tests", "该语言的源码和用例数量尚未接通"})
			sourceComplete, testsComplete = false, false
			continue
		}
		file := after.Files[change.Path]
		if file == nil || file.Kind.TestData || !file.Kind.Source {
			continue
		}
		facts := newFacts[change.Path]
		if facts.sourceUnknown != "" {
			result.Unknown = append(result.Unknown, unknown{change.Path, "source", facts.sourceUnknown})
			sourceComplete = false
			continue
		}
		if file.Kind.Test {
			continue
		}
		count := 0
		old := before.Files[change.BeforePath]
		if old == nil || old.Kind.Test || old.Kind.TestData || !regular(old.Mode) {
			for _, source := range facts.sourceLines {
				if source {
					count++
				}
			}
		} else {
			previous := oldFacts[change.BeforePath]
			if previous.sourceUnknown != "" {
				result.Unknown = append(result.Unknown, unknown{change.BeforePath, "source", previous.sourceUnknown})
				sourceComplete = false
				continue
			}
			fragments, err := workspace.Fragments(ctx, change.Before, change.After)
			if err != nil {
				return result, err
			}
			count, err = addedSourceLines(previous.sourceLines, facts.sourceLines, fragments)
			if err != nil {
				return result, fmt.Errorf("%s: %w", change.Path, err)
			}
		}
		result.SourceLines[change.Path] = count
		sourceCount += count
	}
	for index, state := range []codescan.Snapshot{before, after} {
		facts := []map[string]fileQuantity{oldFacts, newFacts}[index]
		for name, file := range state.Files {
			if !changedPaths[name] {
				continue
			}
			for _, reason := range facts[name].testUnknown {
				result.Unknown = append(result.Unknown, unknown{name, "tests", reason})
				testsComplete = false
			}
			if !file.Kind.Test && file.Kind.Language != "rust" {
				continue
			}
			if !file.CodeComplete || file.Kind.Generated || file.Kind.Dependency {
				result.Unknown = append(result.Unknown, unknown{name, "tests", "变化的测试文件没有完整源码枚举"})
				testsComplete = false
				continue
			}
			identities := map[string]int{}
			for _, test := range file.Tests {
				identities[test.Location()]++
			}
			for _, test := range file.Tests {
				old, oldOK := oldTests[test.Location()]
				current, newOK := newTests[test.Location()]
				if len(test.Unknown) > 0 && (identities[test.Location()] > 1 || !oldOK || !newOK || old.CodeHash != current.CodeHash) {
					result.Unknown = append(result.Unknown, unknown{name, "tests", "变化的用例无法完整枚举: " + test.Name})
					testsComplete = false
				}
			}
			if !file.Operations["tests"].Complete && len(file.Tests) == 0 {
				result.Unknown = append(result.Unknown, unknown{name, "tests", "变化的测试范围未能完整枚举"})
				testsComplete = false
			}
		}
	}
	for location := range newTests {
		if _, exists := oldTests[location]; !exists && !migrated[location] {
			result.AddedTests = append(result.AddedTests, location)
		}
	}
	sort.Strings(result.AddedTests)
	sort.Slice(result.Unknown, func(i, j int) bool {
		if result.Unknown[i].File != result.Unknown[j].File {
			return result.Unknown[i].File < result.Unknown[j].File
		}
		return result.Unknown[i].Metric+result.Unknown[i].Reason < result.Unknown[j].Metric+result.Unknown[j].Reason
	})
	if sourceComplete {
		result.NonTestAddedLines = &sourceCount
	}
	if testsComplete {
		count := len(result.AddedTests)
		result.AddedTestCount = &count
	}
	return result, nil
}

func main() {
	repository := flag.String("repository", "", "repository containing the fixed tree objects")
	base := flag.String("base-tree", "", "full base tree identity")
	tree := flag.String("tree", "", "full candidate or reference tree identity")
	referenceRepository := flag.String("reference-repository", "", "repository containing the reference objects")
	referenceTree := flag.String("reference-tree", "", "full reference tree identity")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	candidate, err := measure(ctx, *repository, *base, *tree)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	reference := candidate
	if *referenceRepository != *repository || *referenceTree != *tree {
		reference, err = measure(ctx, *referenceRepository, *base, *referenceTree)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(ratios(candidate, reference)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
