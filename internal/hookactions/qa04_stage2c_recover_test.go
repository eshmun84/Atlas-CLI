//go:build unix && qa04_physical

// Physical scheduler recovery against one installed Atlas binary.
// Excluded from go test ./... because it requires that binary and signals
// the process group it starts.
//
//	go test -tags qa04_physical ./internal/hookactions -run TestQA04_Stage2C_WorkerInterruptRecovery -count=1

package hookactions_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
)

const (
	stage2CAtlasBin = "/Users/eshmun84/.local/bin/atlas"
	stage2CAtlasSHA = "ffc6ad0d9120ddb2d6a732765090792596e2ba2f9ae2cebb4b046289a97a3081"
	stage2COpenCode = 97257
)

// TestQA04_Stage2C_WorkerInterruptRecovery kills one installed-atlas scheduler
// after that process is observed holding maintain.lock, then waits for the
// persisted owner lease and lets a second installed-atlas scheduler drain the
// same pending generation. Pending YAML is only mutated by Atlas APIs.
func TestQA04_Stage2C_WorkerInterruptRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stage 2C process-death flock check is implemented for unix")
	}
	assertInstalledAtlas(t)
	if _, err := exec.LookPath("codegraph"); err != nil {
		t.Fatal("codegraph is not on PATH")
	}

	ref := c2bRef(t, "qa04-s2c-recover")
	writeIndexableTree(t, ref.Root, 6000)
	hookactions.SetTriggersEnabled(true)

	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	res1, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res1.Reserved {
		t.Fatalf("reserve S1: %+v err=%v", res1, err)
	}
	before, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if before.PendingGeneration != 1 || before.ProcessedGeneration != 0 {
		t.Fatalf("before start pending=%d processed=%d", before.PendingGeneration, before.ProcessedGeneration)
	}

	first := startScheduler(t, ref, res1.Token, res1.Fence)
	t.Cleanup(func() { stopIfAlive(t, first, ref.Home) })

	waitUntilLockHeld(t, ref, first)

	held, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	running, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok {
		t.Fatalf("maintain-last during hold ok=%v err=%v", ok, err)
	}
	if running.Status != hookactions.MaintainStatusRunning || running.PID != first.cmd.Process.Pid {
		t.Fatalf("maintain-last status=%s pid=%d want running pid=%d", running.Status, running.PID, first.cmd.Process.Pid)
	}
	if held.PendingGeneration != 1 || held.ProcessedGeneration != 0 {
		t.Fatalf("during hold pending=%d processed=%d", held.PendingGeneration, held.ProcessedGeneration)
	}
	if held.OwnerToken != res1.Token || held.OwnerFence != res1.Fence {
		t.Fatalf("owner changed during hold fence=%d", held.OwnerFence)
	}
	if held.OwnerState != "running_maintain" {
		t.Fatalf("owner_state=%q want running_maintain", held.OwnerState)
	}
	t.Logf("interrupt pid=%d fence=%d pending=%d processed=%d lease_until=%s",
		first.cmd.Process.Pid, held.OwnerFence, held.PendingGeneration, held.ProcessedGeneration, held.OwnerLeaseUntil)

	killDisposable(t, first, ref.Home)
	waitExit(t, first)

	releaseProbe := acquireAndRelease(t, ref)
	if !releaseProbe {
		t.Fatal("flock still busy after the disposable scheduler died")
	}

	afterKill, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if afterKill.PendingGeneration != held.PendingGeneration || afterKill.ProcessedGeneration != 0 {
		t.Fatalf("after kill pending=%d processed=%d", afterKill.PendingGeneration, afterKill.ProcessedGeneration)
	}
	if afterKill.OwnerToken != res1.Token || afterKill.OwnerFence != res1.Fence {
		t.Fatalf("owner lost before reclaim fence=%d", afterKill.OwnerFence)
	}
	interrupted, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok {
		t.Fatalf("maintain-last after kill ok=%v err=%v", ok, err)
	}
	if interrupted.Status != hookactions.MaintainStatusRunning || interrupted.FinishedAt != "" {
		t.Fatalf("interrupted record status=%s finished=%q", interrupted.Status, interrupted.FinishedAt)
	}

	waitPastLease(t, afterKill.OwnerLeaseUntil)

	res2, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res2.Reserved {
		t.Fatalf("reserve S2: %+v err=%v", res2, err)
	}
	if res2.Fence <= res1.Fence {
		t.Fatalf("S2 fence=%d want > %d", res2.Fence, res1.Fence)
	}
	stale, err := hookactions.CommitGeneration(ref.Home, ref.ProjectID, res1.Token, res1.Fence, held.PendingGeneration, true, hookactions.MaintainErrNone)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Applied {
		t.Fatal("stale S1 commit was applied")
	}
	cleared, err := hookactions.ClearOwnReservation(ref.Home, ref.ProjectID, res1.Token, res1.Fence)
	if err != nil {
		t.Fatal(err)
	}
	if cleared {
		t.Fatal("stale S1 cleared S2")
	}
	mid, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if mid.PendingGeneration != 1 || mid.ProcessedGeneration != 0 {
		t.Fatalf("after stale commit pending=%d processed=%d", mid.PendingGeneration, mid.ProcessedGeneration)
	}
	if mid.OwnerToken != res2.Token || mid.OwnerFence != res2.Fence {
		t.Fatalf("S2 ownership lost token fence=%d", mid.OwnerFence)
	}
	t.Logf("recovered fence=%d pending=%d processed=%d", res2.Fence, mid.PendingGeneration, mid.ProcessedGeneration)

	second := startScheduler(t, ref, res2.Token, res2.Fence)
	t.Cleanup(func() { stopIfAlive(t, second, ref.Home) })
	waitExit(t, second)
	if second.cmd.ProcessState == nil || !second.cmd.ProcessState.Success() {
		t.Fatalf("recovery scheduler exit=%v stderr=%s", second.cmd.ProcessState, second.out.String())
	}

	final, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if final.PendingGeneration != final.ProcessedGeneration || final.ProcessedGeneration != 1 {
		t.Fatalf("final pending=%d processed=%d", final.PendingGeneration, final.ProcessedGeneration)
	}
	if final.OwnerToken != "" || final.OwnerState != "none" {
		t.Fatalf("owner still live token=%q state=%q", final.OwnerToken, final.OwnerState)
	}
	if final.OwnerFence != res2.Fence {
		t.Fatalf("final fence=%d want %d", final.OwnerFence, res2.Fence)
	}

	last, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok {
		t.Fatalf("final maintain-last ok=%v err=%v", ok, err)
	}
	if last.Status != hookactions.MaintainStatusComplete || last.Trigger != "post_code" || last.Generation != 1 {
		t.Fatalf("final maintain-last status=%s trigger=%s generation=%d", last.Status, last.Trigger, last.Generation)
	}
	if last.CEOutcome != hookactions.MaintainOutcomeOK || last.CIOutcome != hookactions.MaintainOutcomeOK || last.ErrorCode != hookactions.MaintainErrNone {
		t.Fatalf("final outcomes ce=%s ci=%s err=%s", last.CEOutcome, last.CIOutcome, last.ErrorCode)
	}
	if last.PID == first.cmd.Process.Pid {
		t.Fatalf("final maintain-last still attributes pid %d", last.PID)
	}

	meta := readCodeGraphMeta(t, ref)
	if meta.NodesTotal <= 0 || meta.FilesTotal <= 0 {
		t.Fatalf("codegraph nodes=%d files=%d", meta.NodesTotal, meta.FilesTotal)
	}
	refreshed, err := time.Parse(time.RFC3339, meta.RefreshedAt)
	if err != nil {
		t.Fatal(err)
	}
	killedAt, err := time.Parse(time.RFC3339Nano, running.StartedAt)
	if err != nil {
		killedAt, err = time.Parse(time.RFC3339, running.StartedAt)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !refreshed.After(killedAt) {
		t.Fatalf("codegraph refreshed %s is not after interrupted start %s", meta.RefreshedAt, running.StartedAt)
	}
	t.Logf("final pending=%d processed=%d ce=%s ci=%s nodes=%d files=%d refreshed=%s",
		final.PendingGeneration, final.ProcessedGeneration, last.CEOutcome, last.CIOutcome, meta.NodesTotal, meta.FilesTotal, meta.RefreshedAt)
}

func assertInstalledAtlas(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile(stage2CAtlasBin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != stage2CAtlasSHA {
		t.Fatalf("installed atlas hash=%s", hex.EncodeToString(sum[:]))
	}
}

func writeIndexableTree(t *testing.T, root string, n int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/qa04s2c\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		dir := filepath.Join(root, "pkg", fmt.Sprintf("p%04d", i/4))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, fmt.Sprintf("f%d.go", i%4))
		body := fmt.Sprintf("package p%04d\n\nfunc F%d(n int) int { return n + %d }\n", i/4, i, i)
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type schedProc struct {
	cmd  *exec.Cmd
	out  *bytes.Buffer
	done chan struct{}
}

func startScheduler(t *testing.T, ref hookactions.ProjectRef, token string, fence int64) *schedProc {
	t.Helper()
	var out bytes.Buffer
	cmd := exec.Command(stage2CAtlasBin)
	cmd.Dir = ref.Root
	cmd.Env = schedulerEnv(ref, token, fence)
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if cmd.Process == nil || cmd.Process.Pid == stage2COpenCode || cmd.Process.Pid <= 1 {
		t.Fatalf("refusing pid %v", cmd.Process)
	}
	proc := &schedProc{cmd: cmd, out: &out, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(proc.done)
	}()
	return proc
}

func schedulerEnv(ref hookactions.ProjectRef, token string, fence int64) []string {
	drop := map[string]bool{
		"ATLAS_HOME":                      true,
		"ATLAS_INTERNAL_MAINTAIN":         true,
		"ATLAS_INTERNAL_MAINTAIN_MODE":    true,
		"ATLAS_INTERNAL_MAINTAIN_ROOT":    true,
		"ATLAS_INTERNAL_MAINTAIN_HOME":    true,
		"ATLAS_INTERNAL_MAINTAIN_TOKEN":   true,
		"ATLAS_INTERNAL_MAINTAIN_FENCE":   true,
		"ATLAS_INTERNAL_MAINTAIN_SESSION": true,
	}
	var env []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if drop[key] {
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"ATLAS_HOME="+ref.Home,
		"ATLAS_INTERNAL_MAINTAIN=1",
		"ATLAS_INTERNAL_MAINTAIN_MODE=post_code_scheduler",
		"ATLAS_INTERNAL_MAINTAIN_ROOT="+ref.Root,
		"ATLAS_INTERNAL_MAINTAIN_HOME="+ref.Home,
		"ATLAS_INTERNAL_MAINTAIN_TOKEN="+token,
		"ATLAS_INTERNAL_MAINTAIN_FENCE="+strconv.FormatInt(fence, 10),
	)
}

func waitUntilLockHeld(t *testing.T, ref hookactions.ProjectRef, proc *schedProc) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if proc.exited() {
			t.Fatalf("scheduler exited before the lock was observed: %s", proc.out.String())
		}
		rec, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
		if err != nil {
			t.Fatal(err)
		}
		busy := lockBusy(t, ref)
		if ok && rec.Status == hookactions.MaintainStatusRunning && rec.PID == proc.cmd.Process.Pid && busy {
			if !processBelongsToHome(t, proc.cmd.Process.Pid, ref.Home) {
				t.Fatalf("pid %d is not the disposable scheduler", proc.cmd.Process.Pid)
			}
			return
		}
		if ok && rec.Status == hookactions.MaintainStatusComplete {
			t.Fatal("maintain completed before the interruption could be observed")
		}
		runtime.Gosched()
	}
	t.Fatal("timed out waiting for the disposable scheduler to hold maintain.lock")
}

func (p *schedProc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func lockBusy(t *testing.T, ref hookactions.ProjectRef) bool {
	t.Helper()
	f, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID)
	if hookactions.IsMaintainBusyForTest(err) {
		return true
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := hookactions.ReleaseMaintainLock(f); err != nil {
		t.Fatal(err)
	}
	return false
}

func acquireAndRelease(t *testing.T, ref hookactions.ProjectRef) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID)
		if err == nil {
			if rel := hookactions.ReleaseMaintainLock(f); rel != nil {
				t.Fatal(rel)
			}
			return true
		}
		if !hookactions.IsMaintainBusyForTest(err) {
			t.Fatal(err)
		}
		runtime.Gosched()
	}
	return false
}

func processBelongsToHome(t *testing.T, pid int, home string) bool {
	t.Helper()
	if pid == stage2COpenCode || pid <= 1 {
		return false
	}
	out, err := exec.Command("ps", "eww", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("ps %d: %v", pid, err)
	}
	text := string(out)
	return strings.Contains(text, stage2CAtlasBin) &&
		strings.Contains(text, "ATLAS_INTERNAL_MAINTAIN_HOME="+home) &&
		strings.Contains(text, "ATLAS_INTERNAL_MAINTAIN_MODE=post_code_scheduler")
}

func killDisposable(t *testing.T, proc *schedProc, home string) {
	t.Helper()
	pid := proc.cmd.Process.Pid
	if !processBelongsToHome(t, pid, home) {
		t.Fatalf("refusing to signal pid %d", pid)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !proc.exited() {
		t.Fatalf("kill group %d: %v", pid, err)
	}
}

func waitExit(t *testing.T, proc *schedProc) {
	t.Helper()
	select {
	case <-proc.done:
	case <-time.After(3 * time.Minute):
		t.Fatalf("pid %d did not exit", proc.cmd.Process.Pid)
	}
}

func stopIfAlive(t *testing.T, proc *schedProc, home string) {
	t.Helper()
	if proc == nil || proc.cmd.Process == nil || proc.exited() {
		return
	}
	if !processBelongsToHome(t, proc.cmd.Process.Pid, home) {
		return
	}
	_ = syscall.Kill(-proc.cmd.Process.Pid, syscall.SIGKILL)
	select {
	case <-proc.done:
	case <-time.After(5 * time.Second):
	}
}

func waitPastLease(t *testing.T, raw string) {
	t.Helper()
	until, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		until, err = time.Parse(time.RFC3339, strings.TrimSpace(raw))
		if err != nil {
			t.Fatalf("owner_lease_until %q: %v", raw, err)
		}
	}
	target := until.Add(time.Second)
	wait := time.Until(target)
	if wait > 6*time.Minute {
		t.Fatalf("lease wait %s exceeds the safety cap", wait)
	}
	if wait > 0 {
		t.Logf("waiting %s for persisted owner lease %s", wait.Round(time.Second), until.Format(time.RFC3339Nano))
		time.Sleep(wait)
	}
}

type codeGraphMeta struct {
	NodesTotal  int    `json:"nodesTotal"`
	FilesTotal  int    `json:"filesTotal"`
	RefreshedAt string `json:"refreshedAt"`
}

func readCodeGraphMeta(t *testing.T, ref hookactions.ProjectRef) codeGraphMeta {
	t.Helper()
	path := filepath.Join(ref.Home, "projects", ref.ProjectID, "codegraph", "metadata.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var meta codeGraphMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}
