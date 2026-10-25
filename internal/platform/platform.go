package platform

import (
	"os"
	"path/filepath"
)

type Env struct {
	HomeDir string
	DataDir string
}

func Current() (Env, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	return Env{HomeDir: dir, DataDir: filepath.Join(dir, ".taskbelay")}, nil
}
