//go:build linux || freebsd

package native

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	protest "github.com/go-delve/delve/pkg/proc/test"
)

// TestWaitForForkExec verifies that a PID observed with its inherited,
// pre-exec command line remains eligible for later scans.
func TestWaitForForkExec(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "waitfor-forkexec")
	source := filepath.Join(protest.FindFixturesDir(), "waitfor_forkexec.c")
	if out, err := exec.Command("cc", "-o", helper, source).CombinedOutput(); err != nil {
		t.Fatalf("building helper: %v\n%s", err, out)
	}

	uniqueArg := fmt.Sprintf("%s-%d", t.Name(), os.Getpid())
	cmd := exec.Command(helper, "--launch", helper, "--target", uniqueArg)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := 0
	t.Cleanup(func() {
		if pid != 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	pidLine, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("reading child PID: %v", err)
	}
	pid, err = strconv.Atoi(strings.TrimSpace(pidLine))
	if err != nil {
		t.Fatalf("parsing child PID %q: %v", pidLine, err)
	}

	prefix := helper + " --target " + uniqueArg
	seen := make(map[int]struct{})
	// The fixture has reported the child only after WUNTRACED observed its
	// SIGSTOP, so this scan deterministically sees the child before exec.
	found, err := waitForSearchProcess(prefix, seen)
	if err != nil {
		t.Fatal(err)
	}
	if found != 0 {
		t.Fatalf("found pre-exec child %d", found)
	}
	if _, ok := seen[pid]; ok {
		t.Fatalf("pre-exec child %d was cached", pid)
	}

	// Resuming the child makes it exec the same fixture in target mode. The PID
	// and seen map are unchanged, so finding it requires retrying that PID.
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		t.Fatalf("resuming child: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		found, err = waitForSearchProcess(prefix, seen)
		if err != nil {
			t.Fatal(err)
		}
		if found == pid {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("did not find child %d after exec", pid)
}
