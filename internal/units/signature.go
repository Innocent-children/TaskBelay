package units

import (
	"context"
	"hash/fnv"
	"math"
	"sort"
	"strings"
)

func Grams(ctx context.Context, words []string) ([]uint64, error) {
	set := map[uint64]bool{}
	width := min(GramSize, len(words))
	for i := 0; width > 0 && i+width <= len(words); i++ {
		if i%PollBytes == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		h := fnv.New64a()
		h.Write([]byte(strings.Join(words[i:i+width], "\x00")))
		set[h.Sum64()] = true
	}
	out := make([]uint64, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func Winnow(ctx context.Context, words []string) ([]uint64, error) {
	var hashes []uint64
	width := min(GramSize, len(words))
	for i := 0; width > 0 && i+width <= len(words); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		h := fnv.New64a()
		h.Write([]byte(strings.Join(words[i:i+width], "\x00")))
		hashes = append(hashes, h.Sum64())
	}
	var out []uint64
	last := -1
	window := min(WindowSize, len(hashes))
	for i := 0; window > 0 && i+window <= len(hashes); i++ {
		at := i
		for j := i; j < i+window; j++ {
			if hashes[j] <= hashes[at] {
				at = j
			}
		}
		if at != last {
			out = append(out, hashes[at])
			last = at
		}
	}
	return out, nil
}

func MinHash(ctx context.Context, grams []uint64) ([]uint64, error) {
	out := make([]uint64, SignatureSize)
	for i := range out {
		out[i] = math.MaxUint64
		for _, g := range grams {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			x := g + uint64(i+1)*0x9e3779b97f4a7c15
			x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
			x = (x ^ (x >> 27)) * 0x94d049bb133111eb
			x ^= x >> 31
			if x < out[i] {
				out[i] = x
			}
		}
	}
	return out, nil
}

func Similarity(a, b []uint64) float64 {
	i, j, intersection := 0, 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			intersection++
			i++
			j++
		} else if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
