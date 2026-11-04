package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

/**
 * 完整 tree 保留全部内容；文件表只发布本次核对的目标，历史路径按观察编号查询。
 */
func (s *Store) ObservationFile(ctx context.Context, workspace string, id int64, path string) (gitstate.File, error) {
	var file gitstate.File
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		file, err = observationFile(ctx, q, workspace, id, path)
		return err
	})
	return file, err
}

func observationFile(ctx context.Context, q queryer, workspace string, id int64, path string) (gitstate.File, error) {
	var file gitstate.File
	var indexed bool
	if err := q.QueryRowContext(ctx, `SELECT paths_indexed FROM observations_v12 WHERE workspace_id=? AND id=?`, workspace, id).Scan(&indexed); err != nil {
		return file, fmt.Errorf("观察 %d 不属于当前工作区或已不存在: %w", id, err)
	}
	if !indexed {
		return historicalObservationFile(ctx, q, workspace, id, path)
	}
	err := q.QueryRowContext(ctx, `SELECT blob,mode FROM observation_paths_v12 WHERE workspace_id=? AND path=? AND observation_id<=? ORDER BY observation_id DESC LIMIT 1`, workspace, path, id).Scan(&file.Blob, &file.Mode)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return file, err
}

func (s *Store) ObservationMetadata(ctx context.Context, id int64) (Observation, error) {
	var observation Observation
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		observation, err = readObservation(ctx, q, id, false)
		return err
	})
	return observation, err
}

/**
 * 返回最近完整观察和之后单独核对过的路径。恢复到原内容的外部修改也必须再次核对。
 */
func (s *Store) FullObservationState(ctx context.Context, workspace string) (Observation, []string, error) {
	var observation Observation
	var paths []string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var id int64
		err := q.QueryRowContext(ctx, `SELECT id FROM observations_v12 WHERE workspace_id=? AND json_extract(data,'$.Full')=1 AND json_extract(data,'$.Complete')=1 ORDER BY id DESC LIMIT 1`, workspace).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if id > 0 {
			observation, err = readObservation(ctx, q, id, false)
			if err != nil {
				return err
			}
		}
		rows, err := q.QueryContext(ctx, `SELECT path FROM observed_files_v12 WHERE workspace_id=? AND observation_id>?`, workspace, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return err
			}
			paths = append(paths, path)
		}
		return rows.Err()
	})
	return observation, paths, err
}

/**
 * schema 11 的历史文件表只在明确回读该观察时解压，不让初始化遍历全部历史正文。
 */
func historicalObservationFile(ctx context.Context, q queryer, workspace string, id int64, path string) (gitstate.File, error) {
	for id > 0 {
		observation, err := readObservation(ctx, q, id)
		if err != nil {
			return gitstate.File{}, err
		}
		if file, ok := observation.Files[path]; ok {
			return file, nil
		}
		if observation.Full && observation.Complete {
			return gitstate.File{}, nil
		}
		if err = q.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM observations_v12 WHERE workspace_id=? AND id<?`, workspace, id).Scan(&id); err != nil {
			return gitstate.File{}, err
		}
	}
	return gitstate.File{}, nil
}
