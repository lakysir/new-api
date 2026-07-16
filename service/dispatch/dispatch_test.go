package dispatch

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&model.Order{}, &model.OrderPriceSnapshot{}, &model.ScriptVersion{}, &model.UserScript{},
		&model.Node{}, &model.NodeCapability{}, &model.NodeSiteStatus{}, &model.Lease{}, &model.TaskAttempt{},
		&model.OutboxEvent{}, &model.ProcessedEvent{},
	); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// seedScriptVersion publishes an executable version.
func seedScriptVersion(t *testing.T, scriptId, authorId int) int {
	t.Helper()
	v := &model.ScriptVersion{ScriptId: scriptId, AuthorId: authorId, CodeSha256: "sha256:x", ReviewStatus: model.ScriptVersionApproved}
	if err := model.CreateScriptVersion(v); err != nil {
		t.Fatal(err)
	}
	return v.Version
}

// seedNodeWithCapability creates an online IDLE node with an active, tested
// capability for the script version at the given price.
func seedNodeWithCapability(t *testing.T, nodeId string, userId, scriptId, version int, priceMicros int64) {
	t.Helper()
	if err := model.DB.Create(&model.Node{
		Id: nodeId, DeviceId: "d-" + nodeId, UserId: userId,
		State: model.NodeStateIdle, Enabled: true, LastSeenAt: time.Now().Unix(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.NodeCapability{
		NodeId: nodeId, ScriptId: scriptId, Version: version, UserId: userId,
		PriceMicros: priceMicros, DailyQuota: 100, RemainingQuota: 100,
		Status: model.CapabilityStatusActive, TestExpiresAt: time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatal(err)
	}
}

// seedFundedOrder creates an order already in FUNDS_RESERVED.
func seedFundedOrder(t *testing.T, clientId, scriptId, version int, maxMicros int64, key string) *model.Order {
	t.Helper()
	o := &model.Order{
		Id: model.NewOrderId(), ClientId: clientId, ScriptId: scriptId, Version: version,
		State: model.OrderCreated, IdempotencyKey: key, MaxAmountMicros: maxMicros, InputHash: "sha256:in",
	}
	snap := &model.OrderPriceSnapshot{Currency: "USD_TEST", MaxCustomerAmountMicros: maxMicros}
	created, _, err := model.CreateOrderWithSnapshot(o, snap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := model.ApplyTransition(created.Id, model.OrderFundsReserved, nil); err != nil {
		t.Fatal(err)
	}
	return created
}

func TestDispatchAtomicReserveOfferTransition(t *testing.T) {
	scriptId, author := 9001, 100
	v := seedScriptVersion(t, scriptId, author)
	seedNodeWithCapability(t, "dn1", 200, scriptId, v, 100000)
	o := seedFundedOrder(t, 300, scriptId, v, 120000, "disp-1")

	res, err := Dispatch(o.Id, 1)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.NodeId != "dn1" || res.LeaseId == "" || res.EventId == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	// Order advanced to OFFERED.
	got, _ := model.GetOrder(o.Id)
	if got.State != model.OrderOffered {
		t.Fatalf("order should be OFFERED, got %s", got.State)
	}
	// Node is BUSY with an active lease.
	lease, _ := model.GetActiveLeaseForNode("dn1")
	if lease == nil {
		t.Fatal("node should hold an active lease")
	}
	// A task.offer event was enqueued atomically.
	events, _ := model.FetchUnpublished(100)
	found := false
	for _, e := range events {
		if e.EventId == res.EventId && e.Type == "task.offer" {
			found = true
		}
	}
	if !found {
		t.Fatal("task.offer event must be enqueued in the same transaction")
	}
}

func TestDispatchNoCandidates(t *testing.T) {
	scriptId := 9002
	v := seedScriptVersion(t, scriptId, 100)
	// Node price above the order max -> filtered out.
	seedNodeWithCapability(t, "dn2", 200, scriptId, v, 500000)
	o := seedFundedOrder(t, 301, scriptId, v, 120000, "disp-2")
	if _, err := Dispatch(o.Id, 1); err != ErrNoCandidates {
		t.Fatalf("expected ErrNoCandidates, got %v", err)
	}
}

// TestDispatchSkipsBusyCandidateToIdleOne reproduces the "node already has an
// active lease" bug in auto mode: a node can pass the ScheduleCandidates filter
// (state=IDLE) while still carrying a stale active lease (e.g. expired but not
// yet reaped). Dispatch must skip it and reserve the next idle candidate rather
// than failing the whole request.
func TestDispatchSkipsBusyCandidateToIdleOne(t *testing.T) {
	scriptId := 9004
	v := seedScriptVersion(t, scriptId, 100)
	// Two idle candidates. Cheaper node is ranked first so it's attempted first.
	seedNodeWithCapability(t, "dn-stale", 200, scriptId, v, 100000)
	seedNodeWithCapability(t, "dn-free", 201, scriptId, v, 110000)

	// Plant a stale active lease on the top-ranked node while it still shows
	// state=IDLE in the nodes table (the reaper hasn't released it yet).
	active := true
	if err := model.DB.Create(&model.Lease{
		Id: "lea_stale", NodeId: "dn-stale", TaskId: "old-task", Attempt: 1,
		Active: &active, ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	o := seedFundedOrder(t, 500, scriptId, v, 200000, "disp-stale")
	res, err := Dispatch(o.Id, 1)
	if err != nil {
		t.Fatalf("dispatch should fall through to the idle node, got %v", err)
	}
	if res.NodeId != "dn-free" {
		t.Fatalf("expected dispatch to the idle node dn-free, got %s", res.NodeId)
	}
	got, _ := model.GetOrder(o.Id)
	if got.State != model.OrderOffered {
		t.Fatalf("order should be OFFERED, got %s", got.State)
	}
}

func TestConcurrentDispatchSingleNodeOneWinner(t *testing.T) {
	scriptId := 9003
	v := seedScriptVersion(t, scriptId, 100)
	seedNodeWithCapability(t, "dn3", 200, scriptId, v, 100000)

	// Many orders compete for the single node concurrently.
	const n = 30
	orders := make([]string, n)
	for i := 0; i < n; i++ {
		o := seedFundedOrder(t, 400+i, scriptId, v, 120000, "disp-race-"+string(rune('a'+i%26))+string(rune('0'+i/26)))
		orders[i] = o.Id
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for _, id := range orders {
		wg.Add(1)
		go func(orderId string) {
			defer wg.Done()
			if _, err := Dispatch(orderId, 1); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly one dispatch should win the single node, got %d", wins)
	}
	lease, _ := model.GetActiveLeaseForNode("dn3")
	if lease == nil {
		t.Fatal("winning dispatch must hold the lease")
	}
}
