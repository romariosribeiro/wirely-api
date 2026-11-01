package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const callRejectionWindow = 2 * time.Minute

type callRejectionPlan struct {
	caller  types.JID
	message string
}

// Claim before launching a worker so offer/offer_notice duplicates cannot send
// multiple replies. Creator identifies the caller; From may be another device.
func prepareCallRejection(current *session, meta types.BasicCallMeta, now time.Time) (callRejectionPlan, bool) {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.retired || !current.settings.RejectCall || strings.TrimSpace(meta.CallID) == "" || !meta.GroupJID.IsEmpty() {
		return callRejectionPlan{}, false
	}
	caller := meta.CallCreator
	if caller.IsEmpty() {
		caller = meta.From
	}
	caller = caller.ToNonAD()
	if caller.User == "" || (caller.Server != types.DefaultUserServer && caller.Server != types.HiddenUserServer) {
		return callRejectionPlan{}, false
	}
	// Do not reject the instance's own outgoing calls echoed by linked devices.
	if current.client != nil && current.client.Store != nil {
		device := current.client.Store
		if (device.ID != nil && caller == device.ID.ToNonAD()) || (!device.LID.IsEmpty() && caller == device.LID.ToNonAD()) {
			return callRejectionPlan{}, false
		}
	}
	for id, attemptedAt := range current.rejectedCalls {
		if now.Sub(attemptedAt) >= callRejectionWindow {
			delete(current.rejectedCalls, id)
		}
	}
	if _, duplicate := current.rejectedCalls[meta.CallID]; duplicate || len(current.rejectedCalls) >= 256 {
		return callRejectionPlan{}, false
	}
	if current.rejectedCalls == nil {
		current.rejectedCalls = make(map[string]time.Time)
	}
	current.rejectedCalls[meta.CallID] = now
	return callRejectionPlan{caller: caller, message: strings.TrimSpace(current.settings.MsgRejectCall)}, true
}

type callRejector interface {
	RejectCall(context.Context, types.JID, string) error
}

// A successful RejectCall means the signal was sent, not that WhatsApp confirmed
// that every ringing device stopped. Keep that distinction in logs and errors.
func rejectCallAndReply(ctx context.Context, client callRejector, callID string, plan callRejectionPlan, reply func(context.Context, string, string) error) error {
	rejectCtx, cancelReject := context.WithTimeout(ctx, 10*time.Second)
	err := client.RejectCall(rejectCtx, plan.caller, callID)
	cancelReject()
	if err != nil {
		return fmt.Errorf("send call rejection: %w", err)
	}
	if plan.message != "" {
		replyCtx, cancelReply := context.WithTimeout(ctx, 30*time.Second)
		defer cancelReply()
		if err := reply(replyCtx, plan.caller.String(), plan.message); err != nil {
			return fmt.Errorf("call rejection sent but automatic reply failed: %w", err)
		}
	}
	return nil
}

func (m *Manager) rejectIncomingCall(current *session, meta types.BasicCallMeta) {
	plan, ok := prepareCallRejection(current, meta, time.Now())
	if !ok || current.client == nil {
		return
	}
	go func() {
		ctx := current.client.BackgroundEventCtx
		if ctx == nil {
			ctx = context.Background()
		}
		err := rejectCallAndReply(ctx, current.client, meta.CallID, plan, func(ctx context.Context, recipient, message string) error {
			_, err := m.SendText(ctx, current.id, recipient, message)
			return err
		})
		if err != nil {
			slog.Error("automatic call handling failed", "instance_id", current.id, "call_id", meta.CallID, "error", err)
			return
		}
		slog.Info("automatic call rejection requested", "instance_id", current.id, "call_id", meta.CallID, "reply_sent", plan.message != "")
	}()
}
