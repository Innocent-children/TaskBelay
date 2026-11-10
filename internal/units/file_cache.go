package units

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
)

// PackedFile 只保存一份原文；所有词的字节范围相接，单元引用其中连续的一段有效词。
type PackedFile struct {
	File
	TokenSpans []byte
}

// 编号属于 file-spans-v1 的存储格式。
var cachedKinds = [...]string{"symbol", "identifier", "space", "number", "key", "string", "comment"}

func PackFile(ctx context.Context, f File) (PackedFile, error) {
	p := PackedFile{File: f, TokenSpans: make([]byte, 0, len(f.Tokens))}
	for i, t := range f.Tokens {
		if i%PollBytes == 0 && ctx.Err() != nil {
			return PackedFile{}, ctx.Err()
		}
		kind := slices.Index(cachedKinds[:], t.Kind)
		if kind < 0 || t.End <= t.Start {
			return PackedFile{}, fmt.Errorf("无法编码文件词 %q", t.Kind)
		}
		p.TokenSpans = binary.AppendUvarint(p.TokenSpans, uint64(t.End-t.Start)<<3|uint64(kind))
	}
	p.Tokens = nil
	p.Units = slices.Clone(f.Units)
	for i := range p.Units {
		if p.Units[i].Tokens != nil {
			p.Units[i].Tokens = []Token{}
		}
	}
	return p, nil
}

func (p PackedFile) Expand(ctx context.Context) (File, error) {
	f := p.File
	text := strings.Join(f.Lines, "")
	offset := 0
	for i, line := range f.Lines {
		f.Lines[i] = text[offset : offset+len(line)]
		offset += len(line)
	}
	start, line := 0, 1
	if len(p.TokenSpans) > 0 {
		f.Tokens = make([]Token, 0, len(p.TokenSpans))
	}
	for encoded := p.TokenSpans; len(encoded) > 0; {
		if len(f.Tokens)%PollBytes == 0 && ctx.Err() != nil {
			return File{}, ctx.Err()
		}
		span, used := binary.Uvarint(encoded)
		length, kind := span>>3, span&7
		if used <= 0 || length == 0 || length > uint64(len(text)-start) || kind >= uint64(len(cachedKinds)) {
			return File{}, fmt.Errorf("文件词流与正文长度不一致")
		}
		end := start + int(length)
		word := text[start:end]
		f.Tokens = append(f.Tokens, Token{Kind: cachedKinds[kind], Text: word, Start: start, End: end, Line: line})
		line += strings.Count(word, "\n")
		start = end
		encoded = encoded[used:]
	}
	if start != len(text) {
		return File{}, fmt.Errorf("文件词流未覆盖完整正文")
	}
	tokens := Significant(f.Tokens)
	for i := range f.Units {
		if err := ctx.Err(); err != nil {
			return File{}, err
		}
		u := &f.Units[i]
		if u.Start < 0 || u.Start > u.End || u.End > len(text) {
			return File{}, fmt.Errorf("文件单元范围无效")
		}
		if u.Tokens != nil {
			u.Tokens = tokensInRange(tokens, u.Start, u.End)
		}
	}
	return f, ctx.Err()
}
