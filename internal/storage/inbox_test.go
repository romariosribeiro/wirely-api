package storage

import (
	"context"
	"testing"
	"time"
)

func TestInboxListsChatsMessagesAndUnreadState(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Inbox")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	events := []struct {
		id, name, chat, text string
		fromMe               bool
		timestamp            time.Time
	}{
		{"evt_a1", "message.received", "551100000001@s.whatsapp.net", "Primeira", false, now.Add(-4 * time.Minute)},
		{"evt_a2", "message.sent", "551100000001@s.whatsapp.net", "Resposta", true, now.Add(-3 * time.Minute)},
		{"evt_b1", "message.received", "120363000000@g.us", "Grupo", false, now.Add(-2 * time.Minute)},
	}
	for _, event := range events {
		inserted, err := store.SaveActivityEvent(ctx, event.id, instance.ID, event.name, event.timestamp.Format(time.RFC3339Nano), map[string]any{
			"id": "msg_" + event.id, "chat": event.chat, "from": event.chat, "fromMe": event.fromMe,
			"isGroup": event.chat == "120363000000@g.us", "pushName": "Contato", "type": "text", "text": event.text,
		})
		if err != nil || !inserted {
			t.Fatalf("save %s: inserted=%v err=%v", event.id, inserted, err)
		}
	}
	if _, err := store.SaveActivityEvent(ctx, "evt_status_only", instance.ID, "instance.status", now.Format(time.RFC3339Nano), map[string]any{"status": "connected"}); err != nil {
		t.Fatal(err)
	}

	chats, total, err := store.ListChats(ctx, instance.ID, "", 1, 1)
	if err != nil || total != 2 || len(chats) != 1 || chats[0].Chat != "120363000000@g.us" || chats[0].UnreadCount != 1 || !chats[0].IsGroup {
		t.Fatalf("unexpected chats: total=%d items=%#v err=%v", total, chats, err)
	}
	filtered, total, err := store.ListChats(ctx, instance.ID, "551100", 1, 25)
	if err != nil || total != 1 || len(filtered) != 1 || filtered[0].LastMessage.Text != "Resposta" {
		t.Fatalf("unexpected filtered chats: total=%d items=%#v err=%v", total, filtered, err)
	}
	messages, total, err := store.ListChatMessages(ctx, instance.ID, "551100000001@s.whatsapp.net", 1, 25)
	if err != nil || total != 2 || len(messages) != 2 || messages[0].Text != "Resposta" || !messages[0].FromMe {
		t.Fatalf("unexpected messages: total=%d items=%#v err=%v", total, messages, err)
	}
	if err := store.MarkChatRead(ctx, instance.ID, "120363000000@g.us"); err != nil {
		t.Fatal(err)
	}
	chats, _, err = store.ListChats(ctx, instance.ID, "120363", 1, 25)
	if err != nil || len(chats) != 1 || chats[0].UnreadCount != 0 {
		t.Fatalf("mark read failed: %#v %v", chats, err)
	}
	if err := store.PruneActivity(ctx, -time.Hour); err != nil {
		t.Fatal(err)
	}
	var readMarkers int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chat_reads").Scan(&readMarkers); err != nil || readMarkers != 0 {
		t.Fatalf("orphan read markers were not pruned: count=%d err=%v", readMarkers, err)
	}
}

func TestInboxBackfillsExistingActivityEvents(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Backfill")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveActivityEvent(ctx, "evt_backfill", instance.ID, "message.received", time.Now().UTC().Format(time.RFC3339Nano), map[string]any{
		"id": "msg_backfill", "chat": "5511999999999@s.whatsapp.net", "fromMe": false, "isGroup": false, "text": "Antiga",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "DELETE FROM chat_messages"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	messages, total, err := reopened.ListChatMessages(ctx, instance.ID, "5511999999999@s.whatsapp.net", 1, 25)
	if err != nil || total != 1 || len(messages) != 1 || messages[0].Text != "Antiga" {
		t.Fatalf("backfill failed: total=%d items=%#v err=%v", total, messages, err)
	}
}
