package ledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/bits"
	"slices"
	"sort"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// 页正文与目录同时提交，重试相同批次可以复用已有记录。
const indexPageSchema = `
CREATE TABLE analysis_index_pages_v13 (
 seq INTEGER PRIMARY KEY, index_id TEXT NOT NULL, kind TEXT NOT NULL,
 first_key TEXT NOT NULL, last_key TEXT NOT NULL, empty_members INTEGER NOT NULL, legacy INTEGER NOT NULL, sha256 TEXT NOT NULL,
 UNIQUE(index_id,kind,legacy,sha256)
);
CREATE INDEX analysis_page_range_v13 ON analysis_index_pages_v13(index_id,kind,last_key,first_key);
ALTER TABLE analysis_records_v12 ADD COLUMN encoding TEXT NOT NULL DEFAULT '';
ALTER TABLE analysis_records_v12 ADD COLUMN raw_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE analysis_records_v12 ADD COLUMN sha256 TEXT NOT NULL DEFAULT '';
PRAGMA user_version=13;
`

const (
	indexPageKind       = "index-page-v13"
	indexPageRows       = 512
	indexPageBytes      = 64 * 1024
	indexPageCacheBytes = 8 * 1024 * 1024
)

type indexPage struct {
	kind, first, last string
	emptyMembers      bool
	body              storedBody
	part              *storedBody
}

type indexPageCache struct {
	mu    sync.Mutex
	rows  map[string][]IndexRow
	bytes int64
}

type pageBuffers struct {
	raw, compressed []byte
	encoder         *zstd.Encoder
}

var pageBufferPool = sync.Pool{New: func() any {
	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(indexPageBytes), zstd.WithEncoderLevel(zstd.SpeedFastest))
	return &pageBuffers{raw: make([]byte, indexPageBytes), encoder: encoder}
}}

// 小页共用原文和压缩工作区，批次只持有最终需要写入的字节。
func (b *pageBuffers) body(raw []byte) storedBody {
	hash := sha256.Sum256(raw)
	body := storedBody{RawBytes: int64(len(raw)), SHA256: hex.EncodeToString(hash[:]), Encoding: "raw"}
	b.compressed = b.encoder.EncodeAll(raw, b.compressed[:0])
	if len(b.compressed) < len(raw) {
		body.Encoding = "zstd"
		raw = b.compressed
	}
	body.Body = bytes.Clone(raw)
	return body
}

func rowLess(a, b IndexRow) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	return a.Member < b.Member
}

/**
 * 字段按字节长度分隔，保留路径、键和正文里的原始字节，包括 NUL。
 */
func prepareIndexPages(ctx context.Context, rows []IndexRow) ([]indexPage, error) {
	b := pageBufferPool.Get().(*pageBuffers)
	defer pageBufferPool.Put(b)
	if !sort.SliceIsSorted(rows, func(i, j int) bool { return rowLess(rows[i], rows[j]) }) {
		rows = slices.Clone(rows)
		sort.SliceStable(rows, func(i, j int) bool { return rowLess(rows[i], rows[j]) })
	}
	var pages []indexPage
	for len(rows) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, size := 0, 0
		emptyMembers := true
		for n < len(rows) && rows[n].Kind == rows[0].Kind && n < indexPageRows {
			row := rows[n]
			if n > 0 && size+len(row.Key)+len(row.Member)+len(row.Data) > indexPageBytes {
				break
			}
			for _, field := range [...]string{row.Key, row.Member, row.Data} {
				size += len(field) + max(1, (bits.Len(uint(len(field)))+6)/7)
			}
			emptyMembers = emptyMembers && row.Member == ""
			n++
		}
		page := indexPage{kind: rows[0].Kind, first: rows[0].Key, last: rows[n-1].Key, emptyMembers: emptyMembers}
		// 预留格式字节，普通页可直接压缩，不再扩容或复制整页。
		var raw []byte
		if size < len(b.raw) {
			raw = b.raw[:1]
		} else {
			raw = make([]byte, 1, 1+size)
		}
		for _, row := range rows[:n] {
			for _, field := range [...]string{row.Key, row.Member, row.Data} {
				raw = binary.AppendUvarint(raw, uint64(len(field)))
				raw = append(raw, field...)
			}
		}
		if len(raw) <= len(b.raw) {
			page.body = b.body(raw)
			pages = append(pages, page)
			rows = rows[n:]
			continue
		}
		raw, part, err := splitIndexPage(ctx, page.kind, raw, n)
		if err != nil {
			return nil, err
		}
		page.part = part
		body, err := prepareBody(ctx, raw)
		if err != nil {
			return nil, err
		}
		if body.Encoding == "json" {
			body.Encoding = "raw"
		}
		page.body = body
		pages = append(pages, page)
		rows = rows[n:]
	}
	return pages, nil
}

/**
 * 大文件的归属清单和学习摘要常随分类结果重复保存。将两个边界之间的原始字节
 * 和插入位置分别编码，字段缺失或格式改变时仍可直接保存整页。
 */
func splitIndexPage(ctx context.Context, kind string, page []byte, n int) ([]byte, *storedBody, error) {
	raw := page[1:]
	var start, end int
	if n == 1 && len(raw) > indexPageBytes {
		switch kind {
		case "file":
			start = bytes.Index(raw, []byte(`,"Rows":`))
			end = bytes.LastIndex(raw, []byte(`,"References":`))
		case "learning":
			start = bytes.Index(raw, []byte(`,"Lexical":`))
			end = bytes.LastIndex(raw, []byte(`,"Classified":`))
		}
	}
	if start < 0 || end-start < indexPageBytes {
		return page, nil, nil
	}
	part, err := prepareBody(ctx, raw[start:end])
	if err != nil {
		return nil, nil, err
	}
	if part.Encoding == "json" {
		part.Encoding = "raw"
	}
	packed := append([]byte{1}, part.SHA256...)
	packed = binary.AppendUvarint(packed, uint64(start))
	packed = append(packed, raw[:start]...)
	return append(packed, raw[end:]...), &part, nil
}

func savePageBody(ctx context.Context, tx *sql.Tx, b storedBody) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO analysis_cache_v12(kind,cache_key,encoding,raw_bytes,sha256,body)
 VALUES(?,?,?,?,?,?) ON CONFLICT(kind,cache_key) DO NOTHING`, indexPageKind, b.SHA256, b.Encoding, b.RawBytes, b.SHA256, b.Body)
	return err
}

func saveIndexPages(ctx context.Context, tx *sql.Tx, id string, pages []indexPage, legacy bool) error {
	for _, p := range pages {
		if p.part != nil {
			if err := savePageBody(ctx, tx, *p.part); err != nil {
				return err
			}
		}
		if err := savePageBody(ctx, tx, p.body); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_index_pages_v13(index_id,kind,first_key,last_key,empty_members,legacy,sha256)
 VALUES(?,?,?,?,?,?,?) ON CONFLICT(index_id,kind,legacy,sha256) DO NOTHING`, id, p.kind, p.first, p.last, p.emptyMembers, legacy, p.body.SHA256); err != nil {
			return err
		}
	}
	// 最大目录序号在删除后可能被复用，清理用本事务的发布修订判断引用是否变化。
	_, err := tx.ExecContext(ctx, "UPDATE analysis_page_state_v14 SET revision=revision+1")
	return err
}

func unpackIndexPage(raw []byte) ([]IndexRow, error) {
	var rows []IndexRow
	for offset := 0; offset < len(raw); {
		var row IndexRow
		for _, field := range [...]*string{&row.Key, &row.Member, &row.Data} {
			n, used := binary.Uvarint(raw[offset:])
			if used <= 0 || n > uint64(len(raw)-offset-used) {
				return nil, fmt.Errorf("索引页字段长度无效")
			}
			offset += used
			*field = string(raw[offset : offset+int(n)])
			offset += int(n)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Store) readPageBody(ctx context.Context, hash string) ([]byte, error) {
	body := storedBody{SHA256: hash}
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, `SELECT encoding,raw_bytes,body FROM analysis_cache_v12
 WHERE kind=? AND cache_key=? AND sha256=?`, indexPageKind, hash, hash).Scan(&body.Encoding, &body.RawBytes, &body.Body)
	})
	if err != nil {
		return nil, fmt.Errorf("读取索引页 %s: %w", hash, err)
	}
	raw, err := body.decode(ctx)
	if err != nil {
		return nil, fmt.Errorf("还原索引页 %s: %w", hash, err)
	}
	return raw, nil
}

func (s *Store) indexPageRows(ctx context.Context, hash string) ([]IndexRow, error) {
	cache := &s.indexPages
	cache.mu.Lock()
	rows, ok := cache.rows[hash]
	cache.mu.Unlock()
	if ok {
		return rows, ctx.Err()
	}
	raw, err := s.readPageBody(ctx, hash)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || raw[0] > 1 {
		return nil, fmt.Errorf("索引页 %s 格式无效", hash)
	}
	format := raw[0]
	raw = raw[1:]
	if format == 1 {
		if len(raw) < 64 {
			return nil, fmt.Errorf("索引页 %s 缺少共享正文引用", hash)
		}
		partHash := string(raw[:64])
		offset, used := binary.Uvarint(raw[64:])
		if used <= 0 || offset > uint64(len(raw)-64-used) {
			return nil, fmt.Errorf("索引页 %s 共享正文位置无效", hash)
		}
		raw = raw[64+used:]
		part, err := s.readPageBody(ctx, partHash)
		if err != nil {
			return nil, err
		}
		full := make([]byte, 0, len(raw)+len(part))
		full = append(full, raw[:offset]...)
		full = append(full, part...)
		raw = append(full, raw[offset:]...)
	}
	rows, err = unpackIndexPage(raw)
	if err != nil {
		return nil, fmt.Errorf("还原索引页 %s: %w", hash, err)
	}
	size := int64(len(raw)) + int64(len(rows))*64
	if size <= indexPageCacheBytes {
		cache.mu.Lock()
		if cache.rows == nil || cache.bytes+size > indexPageCacheBytes {
			cache.rows = make(map[string][]IndexRow)
			cache.bytes = 0
		}
		if _, exists := cache.rows[hash]; !exists {
			cache.rows[hash] = rows
			cache.bytes += size
		}
		cache.mu.Unlock()
	}
	return rows, ctx.Err()
}
