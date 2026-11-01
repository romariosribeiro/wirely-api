package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/storage"
	"go.mau.fi/whatsmeow"
	whatsmeowStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestPrepareCallRejectionUsesCreatorAndSkipsUnsafeCalls(t *testing.T) {
	caller := types.NewJID("5511888888888", types.DefaultUserServer)
	other := types.NewJID("1234567890", types.HiddenUserServer)
	own := types.NewJID("5511999999999", types.DefaultUserServer)
	ownLID := types.NewJID("9876543210", types.HiddenUserServer)
	for _, tc := range []struct {
		name              string
		meta              types.BasicCallMeta
		disabled, retired bool
		want              types.JID
	}{
		{name: "creator instead of envelope sender", meta: types.BasicCallMeta{CallID: "id", From: other, CallCreator: caller}, want: caller},
		{name: "LID caller", meta: types.BasicCallMeta{CallID: "id", From: caller, CallCreator: other}, want: other},
		{name: "fallback sender", meta: types.BasicCallMeta{CallID: "id", From: caller}, want: caller},
		{name: "disabled", disabled: true, meta: types.BasicCallMeta{CallID: "id", CallCreator: caller}},
		{name: "retired", retired: true, meta: types.BasicCallMeta{CallID: "id", CallCreator: caller}},
		{name: "no call ID", meta: types.BasicCallMeta{CallCreator: caller}},
		{name: "no caller", meta: types.BasicCallMeta{CallID: "id"}},
		{name: "group invitation", meta: types.BasicCallMeta{CallID: "id", CallCreator: caller, GroupJID: types.NewJID("12345", types.GroupServer)}},
		{name: "outgoing phone", meta: types.BasicCallMeta{CallID: "id", From: caller, CallCreator: own}},
		{name: "outgoing LID", meta: types.BasicCallMeta{CallID: "id", From: caller, CallCreator: ownLID}},
		{name: "server identity", meta: types.BasicCallMeta{CallID: "id", From: caller, CallCreator: types.NewJID("server", "server.whatsapp.net")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := &session{settings: storage.InstanceSettings{RejectCall: !tc.disabled, MsgRejectCall: "  Envie uma mensagem  "}, retired: tc.retired,
				client: &whatsmeow.Client{Store: &whatsmeowStore.Device{ID: &own, LID: ownLID}}}
			plan, ok := prepareCallRejection(current, tc.meta, time.Now())
			if ok != !tc.want.IsEmpty() || plan.caller != tc.want {
				t.Fatalf("plan=%#v ok=%v; want caller=%s", plan, ok, tc.want)
			}
			if ok && plan.message != "Envie uma mensagem" {
				t.Fatalf("message not normalized: %q", plan.message)
			}
		})
	}
}

func TestCallRejectionDeduplicatesConcurrentSignalsAndExpires(t *testing.T) {
	current := &session{settings: storage.InstanceSettings{RejectCall: true}}
	meta := types.BasicCallMeta{CallID: "call", CallCreator: types.NewJID("5511888888888", types.DefaultUserServer)}
	now := time.Now()
	var claimed atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if _, ok := prepareCallRejection(current, meta, now); ok {
				claimed.Add(1)
			}
		})
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("duplicate replies possible: %d workers", claimed.Load())
	}
	if _, ok := prepareCallRejection(current, meta, now.Add(callRejectionWindow)); !ok {
		t.Fatal("expired call was never pruned")
	}
	for i := range 255 {
		current.rejectedCalls[fmt.Sprint(i)] = now
	}
	meta.CallID = "overflow"
	if _, ok := prepareCallRejection(current, meta, now); ok {
		t.Fatal("dedupe map is unbounded")
	}
	if _, ok := prepareCallRejection(current, meta, now.Add(2*callRejectionWindow)); !ok {
		t.Fatal("expired entries did not free capacity")
	}
}

type fakeCallRejector struct {
	err    error
	caller types.JID
	callID string
	called bool
}

func (f *fakeCallRejector) RejectCall(ctx context.Context, caller types.JID, id string) error {
	f.called = true
	f.caller = caller
	f.callID = id
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
		return errors.New("rejection needs a bounded deadline")
	}
	return f.err
}

func TestRejectCallAndReplyReportsFailuresAndPreservesOrder(t *testing.T) {
	caller := types.NewJID("1234567890", types.HiddenUserServer)
	failure := errors.New("network failure")
	for _, tc := range []struct {
		name, message       string
		rejectErr, replyErr error
		wantReply           bool
	}{
		{name: "custom reply", message: "Envie uma mensagem", wantReply: true},
		{name: "no reply configured"},
		{name: "rejection fails", message: "reply", rejectErr: failure},
		{name: "reply fails", message: "reply", replyErr: failure, wantReply: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeCallRejector{err: tc.rejectErr}
			replied := false
			err := rejectCallAndReply(context.Background(), client, "call-id", callRejectionPlan{caller: caller, message: tc.message}, func(ctx context.Context, recipient, message string) error {
				replied = true
				if !client.called || recipient != caller.String() || message != tc.message {
					t.Fatal("wrong recipient, message, or reply before rejection")
				}
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
					t.Fatal("reply needs bounded deadline")
				}
				return tc.replyErr
			})
			if client.caller != caller || client.callID != "call-id" || replied != tc.wantReply {
				t.Fatalf("rejection=%#v reply=%v", client, replied)
			}
			if errors.Is(err, failure) != (tc.rejectErr != nil || tc.replyErr != nil) {
				t.Fatalf("error hidden: %v", err)
			}
		})
	}
}

func TestCallOfferNoticeIsReportedAndHandled(t *testing.T) {
	for _, group := range []bool{false, true} {
		t.Run(fmt.Sprintf("group=%v", group), func(t *testing.T) {
			manager := &Manager{}
			// No client means no network action; a claimed ID still verifies the
			// event handler routed a direct notice through the rejection policy.
			current := &session{id: "instance", settings: storage.InstanceSettings{RejectCall: true}}
			ch := make(chan Event, 1)
			manager.SetEventHandler(func(event Event) { ch <- event })
			notice := &events.CallOfferNotice{BasicCallMeta: types.BasicCallMeta{CallID: "call", CallCreator: types.NewJID("5511888888888", types.DefaultUserServer)}, Media: "audio"}
			if group {
				notice.Type = "group"
			}
			manager.handleEvent(current, notice)
			select {
			case event := <-ch:
				if event.Event != "call.offer" || event.Data["source"] != "offer_notice" || event.Data["media"] != "audio" {
					t.Fatalf("missing notice: %#v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("notice was discarded")
			}
			if (len(current.rejectedCalls) > 0) == group {
				t.Fatal("group and direct notice policies mixed")
			}
		})
	}
}
