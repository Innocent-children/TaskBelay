package codescan

import "strings"

func TestMigrations(before, after Snapshot, changes []Change) map[string]string {
	result := map[string]string{}
	oldHash, newHash := map[string][]TestCase{}, map[string][]TestCase{}
	for _, entry := range []struct {
		snapshot Snapshot
		byHash   map[string][]TestCase
	}{{before, oldHash}, {after, newHash}} {
		for _, file := range entry.snapshot.Files {
			for _, test := range file.Tests {
				if len(test.Unknown) == 0 {
					key := test.Framework + "\x00" + test.CodeHash
					entry.byHash[key] = append(entry.byHash[key], test)
				}
			}
		}
	}
	for hash, old := range oldHash {
		current := newHash[hash]
		if len(old) == 1 && len(current) == 1 && old[0].Location() != current[0].Location() {
			result[old[0].Location()] = current[0].Location()
		}
	}
	for _, change := range changes {
		if change.Before == nil || change.After == nil || change.AmbiguousLocation || !change.Before.Test || !change.After.Test {
			continue
		}
		oldFile, newFile := before.Files[change.Before.File], after.Files[change.After.File]
		if oldFile == nil || newFile == nil {
			continue
		}
		for _, old := range oldFile.Tests {
			if old.Declaration != change.Before.ID || len(old.Unknown) > 0 {
				continue
			}
			name := strings.Replace(old.Name, change.Before.Name, change.After.Name, 1)
			if change.Before.Owner != "" && change.Before.Owner != change.After.Owner {
				name = strings.Replace(name, change.Before.Owner, change.After.Owner, 1)
			}
			var matches []string
			for _, current := range newFile.Tests {
				if current.Declaration == change.After.ID && current.Name == name && len(current.Unknown) == 0 {
					matches = append(matches, current.Location())
				}
			}
			if len(matches) == 1 && old.Location() != matches[0] {
				result[old.Location()] = matches[0]
			}
		}
	}
	counts := map[string]int{}
	for _, to := range result {
		counts[to]++
	}
	for from, to := range result {
		if counts[to] > 1 {
			delete(result, from)
		}
	}
	return result
}
