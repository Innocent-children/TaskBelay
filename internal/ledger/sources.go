package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

/**
 * 引用只规整空白，偏移仍指向原文的 UTF-8 字节范围；不截断消息，也不设置最短字数。
 * 否定和限制仍在完整来源中，字符串匹配本身不判断用户授权了什么。
 */
func MatchQuote(source Source, quote string) (Citation, error) {
	if !utf8.ValidString(source.Text) || !utf8.ValidString(quote) {
		return Citation{}, fmt.Errorf("来源和引用须为有效的 UTF-8 文本")
	}
	wanted := strings.Join(strings.Fields(quote), " ")
	if wanted == "" {
		return Citation{}, fmt.Errorf("引用不能为空")
	}
	var normalized strings.Builder
	var starts, ends []int
	pendingStart, pendingEnd := -1, 0
	for i, r := range source.Text {
		end := i + utf8.RuneLen(r)
		if unicode.IsSpace(r) {
			if normalized.Len() > 0 && pendingStart < 0 {
				pendingStart = i
			}
			pendingEnd = end
			continue
		}
		if pendingStart >= 0 {
			normalized.WriteByte(' ')
			starts = append(starts, pendingStart)
			ends = append(ends, pendingEnd)
			pendingStart = -1
		}
		normalized.WriteRune(r)
		for range utf8.RuneLen(r) {
			starts = append(starts, i)
			ends = append(ends, end)
		}
	}
	index := strings.Index(normalized.String(), wanted)
	if index < 0 {
		return Citation{}, fmt.Errorf("引用不能在来源 %s 中找到", source.ID)
	}
	return Citation{SourceID: source.ID, Quote: quote, Start: starts[index], End: ends[index+len(wanted)-1]}, nil
}

func loadSource(ctx context.Context, q queryer, workspace, id string) (Source, error) {
	var source Source
	var data string
	err := q.QueryRowContext(ctx, "SELECT data FROM sources WHERE workspace_id=? AND id=?", workspace, id).Scan(&data)
	if err != nil {
		return source, err
	}
	err = json.Unmarshal([]byte(data), &source)
	return source, err
}

func citation(ctx context.Context, q queryer, workspace, task, sourceID, quote string) (*Citation, error) {
	if sourceID == "" && quote == "" {
		return nil, nil
	}
	if sourceID == "" || quote == "" {
		return nil, fmt.Errorf("原话引用和来源编号须同时提供")
	}
	source, err := loadSource(ctx, q, workspace, sourceID)
	if err != nil {
		return nil, fmt.Errorf("读取引用来源: %w", err)
	}
	if source.TaskID != "" && source.TaskID != task {
		return nil, fmt.Errorf("来源不属于当前任务")
	}
	result, err := MatchQuote(source, quote)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func putSource(ctx context.Context, tx *sql.Tx, source Source) error {
	data, err := json.Marshal(source)
	if err != nil {
		return err
	}
	var existing string
	err = tx.QueryRowContext(ctx, "SELECT data FROM sources WHERE id=?", source.ID).Scan(&existing)
	if err == nil {
		var old Source
		if err := json.Unmarshal([]byte(existing), &old); err != nil {
			return err
		}
		if old.WorkspaceID != source.WorkspaceID || old.TaskID != source.TaskID || old.Kind != source.Kind || old.Text != source.Text || old.Blob != source.Blob || old.Path != source.Path || old.Version != source.Version || !sameCitation(old.Confirmation, source.Confirmation) {
			return fmt.Errorf("来源编号已用于其他内容")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO sources VALUES (?,?,?,?)", source.ID, source.WorkspaceID, source.TaskID, string(data))
	return err
}

/**
 * 项目约定来自固定文件内容；已确认方案由后续方案模块提供确认来源。
 * 此方法不暴露为 MCP 工具，Agent 不能通过它伪造用户消息。
 */
func (s *Store) RecordSource(ctx context.Context, source Source) error {
	if source.ID == "" || source.WorkspaceID == "" || source.Text == "" {
		return fmt.Errorf("来源记录不完整")
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		switch source.Kind {
		case "project_convention":
			if source.Path == "" || source.Blob == "" || source.ObservationID == 0 {
				return fmt.Errorf("项目约定须引用已观察的文件内容")
			}
			observation, err := loadObservation(ctx, tx, source.WorkspaceID, source.ObservationID)
			if err != nil {
				return err
			}
			if observation.Files[source.Path].Blob != source.Blob {
				return fmt.Errorf("约定来源与观察内容不一致")
			}
		case "confirmed_plan":
			if source.TaskID == "" || source.Version < 1 || source.Confirmation == nil {
				return fmt.Errorf("方案来源缺少任务、版本或确认")
			}
			if _, err := loadTask(ctx, tx, source.WorkspaceID, source.TaskID); err != nil {
				return err
			}
			proof, err := citation(ctx, tx, source.WorkspaceID, source.TaskID, source.Confirmation.SourceID, source.Confirmation.Quote)
			if err != nil {
				return err
			}
			origin, err := loadSource(ctx, tx, source.WorkspaceID, proof.SourceID)
			if err != nil {
				return err
			}
			if origin.Kind != "user_message" {
				return fmt.Errorf("方案确认须来自用户消息")
			}
			source.Confirmation = proof
		default:
			return fmt.Errorf("不能由该入口写入此类来源")
		}
		if source.At.IsZero() {
			source.At = time.Now().UTC()
		}
		return putSource(ctx, tx, source)
	})
}

func loadObservation(ctx context.Context, q queryer, workspace string, id int64) (Observation, error) {
	var result Observation
	var data string
	err := q.QueryRowContext(ctx, "SELECT data FROM observations WHERE workspace_id=? AND id=?", workspace, id).Scan(&data)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(data), &result)
	result.ID = id
	return result, err
}

func (s *Store) MessageByEvent(ctx context.Context, workspace string, actor Actor, eventID string) (Message, error) {
	var message Message
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var data string
		if err := q.QueryRowContext(ctx, "SELECT data FROM messages WHERE workspace_id=? AND agent=? AND session_id=? AND event_id=?", workspace, actor.Agent, actor.SessionID, eventID).Scan(&data); err != nil {
			return err
		}
		return json.Unmarshal([]byte(data), &message)
	})
	return message, err
}

func (s *Store) RecordMessage(ctx context.Context, message Message) (Message, error) {
	if err := message.Actor.Validate(); err != nil {
		return Message{}, err
	}
	if message.WorkspaceID == "" || message.EventID == "" || message.ObservationID == 0 {
		return Message{}, fmt.Errorf("消息缺少身份或到达时观察点")
	}
	var result Message
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var data string
		err := tx.QueryRowContext(ctx, "SELECT data FROM messages WHERE workspace_id=? AND agent=? AND session_id=? AND event_id=?", message.WorkspaceID, message.Actor.Agent, message.Actor.SessionID, message.EventID).Scan(&data)
		if err == nil {
			if err := json.Unmarshal([]byte(data), &result); err != nil {
				return err
			}
			if result.Text != message.Text || result.Actor.RoundID != message.Actor.RoundID {
				return fmt.Errorf("消息身份与既有内容冲突")
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		observation, err := loadObservation(ctx, tx, message.WorkspaceID, message.ObservationID)
		if err != nil {
			return err
		}
		if observation.Agent != message.Actor.Agent || observation.SessionID != message.Actor.SessionID || observation.RoundID != message.Actor.RoundID {
			return fmt.Errorf("消息与观察点的来源不一致")
		}
		message.Context = observation.Context
		if message.At.IsZero() {
			message.At = observation.At
		}
		insert, err := tx.ExecContext(ctx, "INSERT INTO messages(workspace_id,agent,session_id,round_id,event_id,observation_id,data) VALUES (?,?,?,?,?,?,?)", message.WorkspaceID, message.Actor.Agent, message.Actor.SessionID, message.Actor.RoundID, message.EventID, message.ObservationID, "")
		if err != nil {
			return err
		}
		message.Seq, err = insert.LastInsertId()
		if err != nil {
			return err
		}
		message.ID = fmt.Sprintf("m%d", message.Seq)
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE messages SET id=?,data=? WHERE seq=?", message.ID, string(encoded), message.Seq); err != nil {
			return err
		}
		if err := putSource(ctx, tx, Source{ID: message.ID, WorkspaceID: message.WorkspaceID, Kind: "user_message", Text: message.Text, ObservationID: message.ObservationID, At: message.At}); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO task_sessions VALUES (?,?,?,?,?,?,?)
ON CONFLICT(workspace_id,agent,session_id) DO UPDATE SET round_id=excluded.round_id,last_message_id=excluded.last_message_id,activity_at=excluded.activity_at`, message.WorkspaceID, message.Actor.Agent, message.Actor.SessionID, message.Actor.RoundID, message.ID, message.At.Format(time.RFC3339Nano), message.At.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		result = message
		return expireRoundDecisions(ctx, tx, message)
	})
	return result, err
}

func (s *Store) ValidateSession(ctx context.Context, workspace string, actor Actor) error {
	if err := actor.Validate(); err != nil {
		return err
	}
	return s.read(ctx, func(ctx context.Context, q queryer) error {
		var round string
		if err := q.QueryRowContext(ctx, "SELECT round_id FROM task_sessions WHERE workspace_id=? AND agent=? AND session_id=?", workspace, actor.Agent, actor.SessionID).Scan(&round); err != nil {
			return fmt.Errorf("当前 session 尚无可核对的消息来源: %w", err)
		}
		if round != actor.RoundID {
			return fmt.Errorf("请求轮次不是该 session 最近记录的轮次")
		}
		return nil
	})
}
