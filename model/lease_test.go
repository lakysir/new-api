package model

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedIdleNode creates an online, IDLE node for lease tests.
func seedIdleNode(t *testing.T, nodeId string) *Node {
	t.Helper()
	n := &Node{
		Id: nodeId, DeviceId: "dev_" + nodeId, UserId: 1,
		State: NodeStateIdle, LastSeenAt: time.Now().Unix(),
	}
	if err := DB.Create(n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReserveNodeSucceedsAndBlocksSecond(t *testing.T) {
	seedIdleNode(t, "node_lease_1")
	_, _, err := ReserveNode("tsk_a", "ord_a", "node_lease_1", 1, time.Minute)
	if err != nil {
		t.Fatalf("first reserve should succeed: %v", err)
	}
	// Node is now BUSY; a second reserve must fail.
	_, _, err = ReserveNode("tsk_b", "ord_b", "node_lease_1", 1, time.Minute)
	if err != ErrNodeBusy {
		t.Fatalf("second reserve must fail with ErrNodeBusy, got %v", err)
	}
}

func TestConcurrentReserveOnlyOneWins(t *testing.T) {
	seedIdleNode(t, "node_lease_2")
	const n = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := ReserveNode("tsk_c", "ord_c", "node_lease_2", i, time.Minute)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("exactly one concurrent reserve must win, got %d", successes)
	}
	// The active-lease invariant: at most one active lease for the node.
	lease, err := GetActiveLeaseForNode("node_lease_2")
	if err != nil {
		t.Fatal(err)
	}
	if lease == nil {
		t.Fatal("expected one active lease")
	}
}

func TestReleaseLeaseFreesNodeForReReserve(t *testing.T) {
	seedIdleNode(t, "node_lease_3")
	lease, _, err := ReserveNode("tsk_d", "ord_d", "node_lease_3", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReleaseLease(lease.Id, "done"); err != nil {
		t.Fatal(err)
	}
	// Node should be IDLE again and reservable.
	if _, _, err := ReserveNode("tsk_e", "ord_e", "node_lease_3", 1, time.Minute); err != nil {
		t.Fatalf("re-reserve after release should succeed: %v", err)
	}
}

func TestReleaseLeaseIdempotent(t *testing.T) {
	seedIdleNode(t, "node_lease_4")
	lease, _, _ := ReserveNode("tsk_f", "ord_f", "node_lease_4", 1, time.Minute)
	if err := ReleaseLease(lease.Id, "done"); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseLease(lease.Id, "done-again"); err != nil {
		t.Fatalf("double release must be a no-op success, got %v", err)
	}
}

func TestIdleHeartbeatDoesNotOverwriteBusyLease(t *testing.T) {
	node := seedIdleNode(t, "node_busy_heartbeat")
	lease, _, err := ReserveNode("tsk_heartbeat", "ord_heartbeat", node.Id, 1, time.Minute)
	require.NoError(t, err)

	require.NoError(t, TouchNodePresence(node.Id, NodeStateIdle))
	busyNode, err := GetNode(node.Id)
	require.NoError(t, err)
	assert.Equal(t, NodeStateBusy, busyNode.State)

	require.NoError(t, ReleaseLease(lease.Id, "done"))
	require.NoError(t, TouchNodePresence(node.Id, NodeStateIdle))
	idleNode, err := GetNode(node.Id)
	require.NoError(t, err)
	assert.Equal(t, NodeStateIdle, idleNode.State)
}

func TestExpireStaleLeases(t *testing.T) {
	seedIdleNode(t, "node_lease_5")
	// Reserve with a lease that is already expired.
	lease, _, err := ReserveNode("tsk_g", "ord_g", "node_lease_5", 1, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_ = lease
	released, err := ExpireStaleLeases(time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if released < 1 {
		t.Fatalf("expected at least one lease released, got %d", released)
	}
	if l, _ := GetActiveLeaseForNode("node_lease_5"); l != nil {
		t.Fatal("expired lease should no longer be active")
	}
}

func TestReserveRejectsOfflineNode(t *testing.T) {
	n := &Node{Id: "node_offline", DeviceId: "d", UserId: 1, State: NodeStateIdle, LastSeenAt: time.Now().Add(-time.Hour).Unix()}
	if err := DB.Create(n).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReserveNode("tsk_h", "ord_h", "node_offline", 1, time.Minute); err != ErrNodeBusy {
		t.Fatalf("offline node must not be reservable, got %v", err)
	}
}
