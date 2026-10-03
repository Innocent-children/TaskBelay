package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

type persistedTask domain.ProcessTask

func encodeTask(task domain.ProcessTask) ([]byte, error) {
	if err := workflow.ValidateProcessTask(task); err != nil {
		return nil, domain.WithExplanation(ErrInvalidArgument, "The Task snapshot cannot be saved because it violates the current process or saved-record rules.")
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(persistedTask(task)); err != nil {
		return nil, domain.WithExplanation(ErrInvalidArgument, "The Task snapshot could not be encoded as JSON.")
	}
	raw := bytes.TrimSuffix(b.Bytes(), []byte("\n"))
	return append([]byte(nil), raw...), nil
}

func decodeArchiveTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil || parsed.Location() != time.UTC {
		return nil, domain.WithExplanation(ErrStorageUnavailable, "The saved archive timestamp is not a valid UTC RFC3339 timestamp.")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}
func decodeTask(raw []byte) (domain.ProcessTask, error) {
	if len(raw) == 0 || !utf8.Valid(raw) || rejectDuplicateJSON(raw) != nil {
		return domain.ProcessTask{}, domain.WithExplanation(ErrStorageUnavailable, "The saved Task snapshot is empty, not UTF-8, malformed or contains duplicate JSON members.")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var dto persistedTask
	if err := d.Decode(&dto); err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(ErrStorageUnavailable, "The saved Task snapshot does not match the current closed JSON structure.")
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return domain.ProcessTask{}, domain.WithExplanation(ErrStorageUnavailable, "The saved Task snapshot contains trailing JSON data.")
	}
	task := domain.ProcessTask(dto)
	if err := workflow.ValidateProcessTask(task); err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(ErrStorageUnavailable, "The saved Task snapshot violates the current process or saved-record rules.")
	}
	return task, nil
}
func rejectDuplicateJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return err
				}
				key := keyToken.(string)
				if seen[key] {
					return fmt.Errorf("duplicate %s", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		if delim == '[' {
			for d.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	return walk()
}
