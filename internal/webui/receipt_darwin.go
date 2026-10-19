//go:build darwin

package webui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func validRuntimeReceiptFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm() == 0o600
}

func processStartIdentity(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid process identity")
	}
	command := exec.Command("/bin/ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	// ps formats lstart using the caller's locale. Persist the same identity
	// when another terminal or the desktop app inspects this process.
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	identity := strings.Join(strings.Fields(string(output)), " ")
	if identity == "" {
		return "", fmt.Errorf("process is unavailable")
	}
	return identity, nil
}
