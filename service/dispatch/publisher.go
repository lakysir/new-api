package dispatch

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// EventSender delivers a control event to a specific node. The real
// implementation is nodehub.Default.Send; tests inject a fake.
type EventSender interface {
	Send(nodeID string, v any) error
}

// resolveTargetNode returns the node a control event should be delivered to.
// For task.* events the aggregate id is the task id; the owning node is the one
// holding the task's attempt. Returns "" when no routing target applies.
func resolveTargetNode(ev model.OutboxEvent) string {
	// The offer payload carries the node explicitly; fall back to the task
	// attempt lookup if absent.
	var p taskOfferPayload
	if json.Unmarshal([]byte(ev.Payload), &p) == nil && p.TaskId != "" {
		ta, err := model.GetTaskAttempt(p.TaskId, p.Attempt)
		if err == nil && ta != nil {
			return ta.NodeId
		}
	}
	return ""
}

// PublishBatch drains up to `limit` unpublished outbox events, delivers each to
// its target node via sender, and marks delivered ones published. An event with
// no resolvable target or an offline node is left unpublished for retry (its
// lease will expire and the order re-matches). Returns counts for observability.
func PublishBatch(sender EventSender, limit int) (delivered int, skipped int, err error) {
	events, err := model.FetchUnpublished(limit)
	if err != nil {
		return 0, 0, err
	}
	for _, ev := range events {
		nodeID := resolveTargetNode(ev)
		if nodeID == "" {
			skipped++
			continue
		}
		if sendErr := sender.Send(nodeID, buildWireEvent(ev)); sendErr != nil {
			// Node offline / write failed: leave unpublished, retry next tick.
			skipped++
			continue
		}
		if merr := model.MarkPublished(ev.EventId); merr != nil {
			return delivered, skipped, merr
		}
		delivered++
	}
	return delivered, skipped, nil
}

// PublishEvent immediately delivers the event created for a just-dispatched
// order. This avoids head-of-line blocking behind old offline-node events.
func PublishEvent(sender EventSender, eventID string) error {
	event, err := model.GetOutboxEvent(eventID)
	if err != nil {
		return err
	}
	if event.PublishedAt > 0 {
		return nil
	}
	nodeID := resolveTargetNode(*event)
	if nodeID == "" {
		return model.ErrNodeNotFound
	}
	if err := sender.Send(nodeID, buildWireEvent(*event)); err != nil {
		return err
	}
	return model.MarkPublished(event.EventId)
}

// buildWireEvent shapes the outbox row into the control-event envelope the
// plugin's ControlChannel expects (type + event_id + payload fields).
func buildWireEvent(ev model.OutboxEvent) map[string]any {
	out := map[string]any{"type": ev.Type, "event_id": ev.EventId}
	var payload map[string]any
	if json.Unmarshal([]byte(ev.Payload), &payload) == nil {
		for k, v := range payload {
			out[k] = v
		}
	}
	return out
}

// StartPublisher runs PublishBatch on an interval until stop is closed. Intended
// to be launched as a background goroutine at server startup.
func StartPublisher(sender EventSender, interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if _, _, err := PublishBatch(sender, 100); err != nil {
				common.SysError("outbox publish batch failed: " + err.Error())
			}
		}
	}
}

// StartLeaseExpiryLoop periodically releases active leases whose TTL has passed.
// A lease is normally released when the provider reports the task's terminal
// state (result_ready / failed / cancelled). If the provider tab closes or the
// relay drops mid-run, that message never arrives and the lease stays active,
// permanently occupying one of the node's concurrency slots (shown as e.g.
// "1/2 slots" on an otherwise idle node). This sweep reclaims those slots once
// DefaultLeaseTTL has elapsed. Runs until stop is closed.
func StartLeaseExpiryLoop(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			released, err := model.ExpireStaleLeases(common.GetTimestamp())
			if err != nil {
				common.SysError("stale lease expiry failed: " + err.Error())
			} else if released > 0 {
				common.SysLog(fmt.Sprintf("released %d stale lease(s)", released))
			}
		}
	}
}
