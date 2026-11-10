package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

const checkPageChars = 16 * 1024

type checkTextKey struct {
	seq, rawBytes  int64
	encoding, sha  string
	metadata, body [sha256.Size]byte
}

type checkText struct {
	total int
	pages []storedBody
}

// 每个进程至多保留一份分块正文。有效性和交付状态仍在每次请求的账本事务中读取。
type checkPageCache struct {
	mu       sync.Mutex
	key      checkTextKey
	text     *checkText
	building map[checkTextKey]chan struct{}
}

func (c *checkPageCache) acquire(ctx context.Context, key checkTextKey) (*checkText, func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		c.mu.Lock()
		if c.text != nil && c.key == key {
			text := c.text
			c.mu.Unlock()
			return text, nil, nil
		}
		done := c.building[key]
		if done == nil {
			if c.building == nil {
				c.building = map[checkTextKey]chan struct{}{}
			}
			done = make(chan struct{})
			c.building[key] = done
			c.mu.Unlock()
			return nil, func() {
				c.mu.Lock()
				delete(c.building, key)
				close(done)
				c.mu.Unlock()
			}, nil
		}
		c.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
}

func runeEnd(data []byte, count int) int {
	offset := 0
	for range count {
		if data[offset] < utf8.RuneSelf {
			offset++
			continue
		}
		_, n := utf8.DecodeRune(data[offset:])
		offset += n
	}
	return offset
}

func pageEnd(total, offset, limit int) (int, error) {
	if offset > total {
		return 0, fmt.Errorf("char_offset 超过正文字符数 %d", total)
	}
	return offset + min(limit, total-offset), nil
}

func textPage(raw []byte, offset, limit int) (string, int, int, error) {
	total := utf8.RuneCount(raw)
	end, err := pageEnd(total, offset, limit)
	if err != nil {
		return "", 0, total, err
	}
	if total == len(raw) {
		return string(raw[offset:end]), end, total, nil
	}
	start := runeEnd(raw, offset)
	return string(raw[start : start+runeEnd(raw[start:], end-offset)]), end, total, nil
}

func (t *checkText) page(ctx context.Context, offset, limit int) (string, int, int, error) {
	end, err := pageEnd(t.total, offset, limit)
	if err != nil {
		return "", 0, t.total, err
	}
	var out strings.Builder
	for at := offset; at < end; {
		block := at / checkPageChars
		raw, err := t.pages[block].decode(ctx)
		if err != nil {
			return "", 0, t.total, err
		}
		n := min(end-at, checkPageChars-at%checkPageChars)
		start := runeEnd(raw, at%checkPageChars)
		out.Write(raw[start : start+runeEnd(raw[start:], n)])
		at += n
	}
	return out.String(), end, t.total, nil
}

func (c *checkPageCache) page(ctx context.Context, id int64, data string, report storedBody, offset, limit int) (string, int, int, error) {
	// 连同数据库里的压缩字节一起核对，正文缺失、被替换或损坏都不能命中旧缓存。
	key := checkTextKey{id, report.RawBytes, report.Encoding, report.SHA256, sha256.Sum256([]byte(data)), sha256.Sum256(report.Body)}
	text, release, err := c.acquire(ctx, key)
	if err != nil {
		return "", 0, 0, err
	}
	if text != nil {
		return text.page(ctx, offset, limit)
	}
	defer release()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		return "", 0, 0, fmt.Errorf("检查 %d: %w", id, err)
	}
	if fields == nil {
		return "", 0, 0, fmt.Errorf("检查 %d 元数据缺失", id)
	}
	if report.Encoding != "" {
		raw, err := report.decode(ctx)
		if err != nil {
			return "", 0, 0, fmt.Errorf("检查 %d 报告: %w", id, err)
		}
		fields["report"] = raw
	} else if len(fields["report"]) == 0 || string(fields["report"]) == "null" {
		return "", 0, 0, fmt.Errorf("检查 %d 报告正文缺失", id)
	}
	// 旧读取路径会验证整个检查记录的字段类型；分页缓存只在首次读取时做同样的验证。
	var metadata struct {
		CheckRecord
		Report json.RawMessage `json:"report"`
	}
	if err := json.Unmarshal([]byte(data), &metadata); err != nil {
		return "", 0, 0, fmt.Errorf("检查 %d: %w", id, err)
	}
	var decoded DeliveryReport
	if err := json.Unmarshal(fields["report"], &decoded); err != nil {
		return "", 0, 0, fmt.Errorf("检查 %d 报告: %w", id, err)
	}
	raw, err := marshalRecordFields("checks", id, fields)
	if err != nil {
		return "", 0, 0, fmt.Errorf("读取 checks %d: %w", id, err)
	}
	body, end, total, err := textPage(raw, offset, limit)
	if err != nil || total <= checkPageChars {
		return body, end, total, err
	}
	// 固定字符数切块，一页请求最多解压两块。压缩后装不下的正文照常分页返回。
	text = &checkText{total: total}
	b := pageBufferPool.Get().(*pageBuffers)
	defer pageBufferPool.Put(b)
	retained := 0
	for at := 0; at < total; at += checkPageChars {
		if err := ctx.Err(); err != nil {
			return "", 0, 0, err
		}
		n := runeEnd(raw, min(checkPageChars, total-at))
		page := b.body(raw[:n])
		retained += len(page.Body) + 192 // 包括块头、摘要和切片
		if retained > indexPageCacheBytes {
			return body, end, total, nil
		}
		text.pages = append(text.pages, page)
		raw = raw[n:]
	}
	c.mu.Lock()
	c.key, c.text = key, text
	c.mu.Unlock()
	return body, end, total, nil
}
