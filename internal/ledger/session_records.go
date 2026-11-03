package ledger

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

/**
 * AppendSessionRecord 只追加当前会话的新条目；身份不完整时不猜测目标文件。
 */
func (s *Store) AppendSessionRecord(workspace string, actor Actor, text string) error {
	if workspace == "" || actor.SessionID == "" || (actor.Agent != "claude" && actor.Agent != "codex") || text == "" {
		return nil
	}
	root, err := os.OpenRoot(s.dataDir)
	if err != nil {
		return err
	}
	defer root.Close()
	workspaceKey := fmt.Sprintf("%x", sha256.Sum256([]byte(workspace)))[:32]
	sessionKey := fmt.Sprintf("%x", sha256.Sum256([]byte(actor.SessionID)))[:32]
	dir := filepath.Join("session-records", workspaceKey)
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := root.OpenFile(filepath.Join(dir, actor.Agent+"-"+sessionKey+".md"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, err := file.Write([]byte(text))
	if err == nil && n != len(text) {
		err = io.ErrShortWrite
	}
	return errors.Join(err, file.Close())
}
