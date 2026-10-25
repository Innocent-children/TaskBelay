package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Innocent-children/taskbelay/skills"
)

const skillMarker = "TaskBelay installed skill\n"

func installSkills(root string) error {
	for _, name := range skills.Names {
		target := filepath.Join(root, name)
		if _, err := os.Lstat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
		if err := os.Mkdir(target, 0o700); err != nil {
			if os.IsExist(err) {
				continue
			}
			return err
		}
		err := fs.WalkDir(skills.Files, name, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(name, path)
			if err != nil {
				return err
			}
			out := filepath.Join(target, rel)
			if entry.IsDir() {
				return os.MkdirAll(out, 0o700)
			}
			data, err := skills.Files.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(out, data, 0o600)
		})
		if err == nil {
			err = os.WriteFile(filepath.Join(target, ".taskbelay"), []byte(skillMarker), 0o600)
		}
		if err != nil {
			return fmt.Errorf("复制 skill %s 未完成（未标记的目录保留供检查）: %w", name, err)
		}
	}
	return nil
}

func uninstallSkills(root string) error {
	for _, name := range skills.Names {
		target := filepath.Join(root, name)
		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		marker := filepath.Join(target, ".taskbelay")
		info, err = os.Lstat(marker)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(marker)
		if err != nil {
			return err
		}
		if string(data) != skillMarker {
			continue
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}
	return nil
}
