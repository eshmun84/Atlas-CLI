package hookactions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// RunWorker executes Context Economy then Code Intelligence maintenance under
// the cross-process maintain lock. Fail-open: CE/CI errors are recorded in
// maintain-last.yaml but do not fail the worker (busy/not-initialized → success).
func RunWorker(ctx context.Context, ref ProjectRef) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, WorkerTimeout)
		defer cancel()
	}

	lock, err := acquireMaintainLockForWorker(ref.Home, ref.ProjectID)
	if err != nil {
		if errors.Is(err, errMaintainBusy) {
			return nil
		}
		return err
	}
	defer func() {
		clearMaintainLease(ref.Home, ref.ProjectID)
		_ = ReleaseMaintainLock(lock)
	}()

	_ = writeMaintainLease(ref.Home, ref.ProjectID, leaseStateRunning)

	sessionID := strings.TrimSpace(ref.SessionID)

	live, err := ResolveAt(ref.Root, ref.Home)
	if err != nil {
		_, started := beginMaintainLast(ref.Home, ref.ProjectID, sessionID)
		finishMaintainLast(ref.Home, ref.ProjectID, sessionID, started, MaintainOutcomeSkipped, MaintainOutcomeSkipped, MaintainErrResolveFailed)
		return nil
	}
	live.SessionID = sessionID

	_, started := beginMaintainLast(live.Home, live.ProjectID, sessionID)

	ce := &ContextEconomyAction{}
	ceRes, ceErr := ce.Maintain(ctx, live)
	ceOut := mapCEOutcome(ceRes, ceErr)

	ciOut := MaintainOutcomePending
	var ciErr error
	if ctx.Err() == nil {
		ci := &CodeIntelAction{}
		var ciRes CodeIntelResult
		ciRes, ciErr = ci.Maintain(ctx, live)
		ciOut = mapCIOutcome(ciRes, ciErr)
	} else {
		ciOut = mapCIOutcome(CodeIntelResult{}, ctx.Err())
	}

	errCode := deriveErrorCode(ceOut, ciOut, ctx.Err())
	finishMaintainLast(live.Home, live.ProjectID, sessionID, started, ceOut, ciOut, errCode)
	return ctx.Err()
}

// MaybeRunInternalMaintain is the process entry for detached workers.
// Returns handled=true when this process is an internal maintain worker.
func MaybeRunInternalMaintain(stdout, stderr io.Writer) (handled bool, err error) {
	if strings.TrimSpace(os.Getenv(EnvInternalMaintain)) != "1" {
		return false, nil
	}
	_ = stdout
	root := strings.TrimSpace(os.Getenv(envMaintainRoot))
	homePath := strings.TrimSpace(os.Getenv(envMaintainHome))
	sessionID := strings.TrimSpace(os.Getenv(envMaintainSession))
	if root == "" {
		fmt.Fprintln(stderr, "hookactions: internal maintain missing root")
		return true, fmt.Errorf("hookactions: internal maintain missing root")
	}
	if homePath != "" {
		_ = os.Setenv("ATLAS_HOME", homePath)
	}
	ref, err := ResolveAt(root, homePath)
	if err != nil {
		return true, nil
	}
	ref.SessionID = sessionID
	ctx, cancel := context.WithTimeout(context.Background(), WorkerTimeout)
	defer cancel()
	_ = RunWorker(ctx, ref)
	return true, nil
}

// InlineSpawner runs maintenance in-process and waits (tests / workers).
// Do not use as the Action admission Spawner under AdmissionTimeout — prefer
// AsyncInlineSpawner so admission stays within adapter subprocess budgets.
func InlineSpawner(ref ProjectRef) error {
	ctx, cancel := context.WithTimeout(context.Background(), WorkerTimeout)
	defer cancel()
	return RunWorker(ctx, ref)
}

var asyncInlineWG sync.WaitGroup

// AsyncInlineSpawner schedules in-process maintenance and returns immediately
// (mirrors detached spawn for tests without exec).
func AsyncInlineSpawner(ref ProjectRef) error {
	asyncInlineWG.Add(1)
	go func() {
		defer asyncInlineWG.Done()
		_ = InlineSpawner(ref)
	}()
	return nil
}

// WaitAsyncInlineForTest blocks until AsyncInlineSpawner work finishes.
func WaitAsyncInlineForTest() {
	asyncInlineWG.Wait()
}
