//go:build darwin

package execution

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestProcessIdentityPreservesPermissionAndExitErrorCauses(t *testing.T) {
	cause := &os.PathError{Op: "fork/exec", Path: "/bin/ps", Err: syscall.EACCES}
	_, e := processIdentityResult(nil, cause)
	if !errors.Is(e, os.ErrPermission) || !strings.Contains(e.Error(), "/bin/ps") {
		t.Fatal("denied query lost original cause", e)
	}
	_, exit := exec.Command("/bin/sh", "-c", "printf 'Operation not permitted' >&2; exit 1").Output()
	_, e = processIdentityResult(nil, exit)
	var retained *exec.ExitError
	if !errors.As(e, &retained) || errors.Is(e, errProcessEnded) || !strings.Contains(e.Error(), "Operation not permitted") {
		t.Fatal("query failure was treated as process death", e)
	}
}
func TestActualOwnerAndEndedIdentityRemainDistinct(t *testing.T) {
	pid, identity, e := Owner()
	if e != nil {
		t.Fatal(e)
	}
	alive, e := OwnerStatus(Run{OwnerPID: pid, OwnerIdentity: identity})
	if e != nil || !alive {
		t.Fatal(alive, e)
	}
	// Empty successful output is absence, not a fabricated process identity.
	_, e = processIdentityResult(nil, nil)
	if !errors.Is(e, errProcessEnded) {
		t.Fatal(e)
	}
}
