package ledger

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

type storedBody struct {
	Encoding string
	RawBytes int64
	SHA256   string
	Body     []byte
}

func prepareBody(ctx context.Context, raw []byte) (storedBody, error) {
	body := storedBody{Encoding: "json", RawBytes: int64(len(raw)), SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Body: raw}
	if err := ctx.Err(); err != nil {
		return body, err
	}
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return body, err
	}
	for offset := 0; offset < len(raw); offset += 32 * 1024 {
		if err := ctx.Err(); err != nil {
			writer.Close()
			return body, err
		}
		if _, err := writer.Write(raw[offset:min(len(raw), offset+32*1024)]); err != nil {
			return body, err
		}
	}
	if err := writer.Close(); err != nil {
		return body, err
	}
	if err := ctx.Err(); err != nil {
		return body, err
	}
	if compressed.Len() < len(raw) {
		body.Encoding, body.Body = "gzip", compressed.Bytes()
	}
	return body, nil
}

func (body storedBody) decode(ctx context.Context) ([]byte, error) {
	if body.RawBytes < 0 {
		return nil, fmt.Errorf("正文长度无效")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw := body.Body
	switch body.Encoding {
	case "json":
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body.Body))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		var out bytes.Buffer
		buffer := make([]byte, 32*1024)
		limited := io.LimitReader(reader, body.RawBytes+1)
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			n, err := limited.Read(buffer)
			out.Write(buffer[:n])
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
		}
		raw = out.Bytes()
	default:
		return nil, fmt.Errorf("未知正文编码 %q", body.Encoding)
	}
	if int64(len(raw)) != body.RawBytes || fmt.Sprintf("%x", sha256.Sum256(raw)) != body.SHA256 {
		return nil, fmt.Errorf("正文长度或摘要不一致")
	}
	return raw, ctx.Err()
}

func prepareObservation(ctx context.Context, observation Observation) ([]byte, *storedBody, error) {
	if observation.Full && observation.Complete && observation.Files == nil {
		return nil, nil, fmt.Errorf("完整观察缺少文件表；空仓库须保存空映射")
	}
	if !observation.Full || !observation.Complete || observation.Files == nil {
		data, err := json.Marshal(observation)
		return data, nil, err
	}
	raw, err := json.Marshal(observation.Files)
	if err != nil {
		return nil, nil, err
	}
	body, err := prepareBody(ctx, raw)
	if err != nil {
		return nil, nil, err
	}
	data, err := json.Marshal(struct {
		*Observation
		Files map[string]gitstate.File `json:"Files,omitempty"`
	}{Observation: &observation})
	return data, &body, err
}

func saveFileSet(ctx context.Context, tx *sql.Tx, body *storedBody) (any, error) {
	if body == nil {
		return nil, nil
	}
	id := "files1:" + body.SHA256
	_, err := tx.ExecContext(ctx, `INSERT INTO file_sets_v11(id,format,encoding,raw_bytes,sha256,body) VALUES(?,1,?,?,?,?) ON CONFLICT(id) DO NOTHING`, id, body.Encoding, body.RawBytes, body.SHA256, body.Body)
	if err != nil {
		return nil, err
	}
	var format int
	var rawBytes int64
	var hash string
	if err := tx.QueryRowContext(ctx, `SELECT format,raw_bytes,sha256 FROM file_sets_v11 WHERE id=?`, id).Scan(&format, &rawBytes, &hash); err != nil {
		return nil, err
	}
	if format != 1 || rawBytes != body.RawBytes || hash != body.SHA256 {
		return nil, fmt.Errorf("文件表 %s 元数据不一致", id)
	}
	return id, nil
}

func readObservation(ctx context.Context, q queryer, id int64) (Observation, error) {
	var result Observation
	var data string
	var filesID sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT data,files_set_id FROM observations_v11 WHERE id=?`, id).Scan(&data, &filesID); err != nil {
		return result, fmt.Errorf("观察 %d: %w", id, err)
	}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return result, fmt.Errorf("观察 %d: %w", id, err)
	}
	result.ID = id
	if filesID.Valid {
		var body storedBody
		var format int
		err := q.QueryRowContext(ctx, `SELECT format,encoding,raw_bytes,sha256,body FROM file_sets_v11 WHERE id=?`, filesID.String).Scan(&format, &body.Encoding, &body.RawBytes, &body.SHA256, &body.Body)
		if err != nil {
			return result, fmt.Errorf("观察 %d 文件表 %s: %w", id, filesID.String, err)
		}
		if format != 1 || filesID.String != "files1:"+body.SHA256 || !result.Full || !result.Complete {
			return result, fmt.Errorf("观察 %d 文件表引用无效", id)
		}
		raw, err := body.decode(ctx)
		if err != nil {
			return result, fmt.Errorf("观察 %d 文件表 %s: %w", id, filesID.String, err)
		}
		if err := json.Unmarshal(raw, &result.Files); err != nil || result.Files == nil {
			return result, fmt.Errorf("观察 %d 文件表正文无效: %v", id, err)
		}
	}
	if result.Full && result.Complete && result.Files == nil {
		return result, fmt.Errorf("观察 %d 文件表正文缺失", id)
	}
	return result, nil
}

func checkMetadata(record CheckRecord) ([]byte, error) {
	return json.Marshal(struct {
		*CheckRecord
		Report any `json:"report,omitempty"`
	}{CheckRecord: &record})
}

func restoreCheckBody(ctx context.Context, q queryer, id int64, data string) (string, error) {
	var body storedBody
	if err := q.QueryRowContext(ctx, `SELECT report_encoding,report_raw_bytes,report_sha256,report_body FROM checks_v11 WHERE seq=?`, id).Scan(&body.Encoding, &body.RawBytes, &body.SHA256, &body.Body); err != nil {
		return "", fmt.Errorf("检查 %d: %w", id, err)
	}
	if body.Encoding == "" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &fields); err != nil {
			return "", fmt.Errorf("检查 %d: %w", id, err)
		}
		if len(fields["report"]) == 0 || string(fields["report"]) == "null" {
			return "", fmt.Errorf("检查 %d 报告正文缺失", id)
		}
		return data, nil
	}
	raw, err := body.decode(ctx)
	if err != nil {
		return "", fmt.Errorf("检查 %d 报告: %w", id, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		return "", err
	}
	fields["report"] = raw
	encoded, err := json.Marshal(fields)
	return string(encoded), err
}
