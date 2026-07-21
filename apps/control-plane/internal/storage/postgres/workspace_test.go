package postgres

import (
	"testing"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/workspace"
)

func TestWorkspaceQuotaDeltaFollowsComputeLifecycle(t *testing.T) {
	if got := workspaceQuotaDelta(workspace.DesiredStopped, workspace.DesiredRunning, 2); got != 2 {
		t.Fatalf("stopped to running delta = %d", got)
	}
	if got := workspaceQuotaDelta(workspace.DesiredRunning, workspace.DesiredStopped, 2); got != -2 {
		t.Fatalf("running to stopped delta = %d", got)
	}
	if got := workspaceQuotaDelta(workspace.DesiredRunning, workspace.DesiredTerminated, 2); got != -2 {
		t.Fatalf("running to terminated delta = %d", got)
	}
	if got := workspaceQuotaDelta(workspace.DesiredStopped, workspace.DesiredTerminated, 2); got != 0 {
		t.Fatalf("stopped to terminated delta = %d", got)
	}
}
