package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// settingValues 是每个设置项允许的取值，第一个是默认值。
var settingValues = map[string][]string{
	"workflow":    {"auto", "semi"},  // 工作模式：全自动、半自动
	"enforcement": {"block", "hint"}, // 拦截强度：拦截一次、只提示
}

// Setting 返回设置项 key 的值，没设置过时返回默认值。
func (l *Ledger) Setting(ctx context.Context, key string) (string, error) {
	allowed, err := settingAllowed(key)
	if err != nil {
		return "", err
	}
	var value string
	err = l.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return allowed[0], nil
	}
	return value, err
}

// SetSetting 把设置项 key 设为 value。value 必须是这一项允许的取值。
func (l *Ledger) SetSetting(ctx context.Context, key, value string) error {
	allowed, err := settingAllowed(key)
	if err != nil {
		return err
	}
	if !slices.Contains(allowed, value) {
		return fmt.Errorf("%s 只能设为 %s", key, strings.Join(allowed, " 或 "))
	}
	_, err = l.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

func settingAllowed(key string) ([]string, error) {
	allowed, ok := settingValues[key]
	if !ok {
		return nil, fmt.Errorf("没有这个设置项：%s", key)
	}
	return allowed, nil
}
