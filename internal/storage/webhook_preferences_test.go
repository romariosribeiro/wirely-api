package storage

import (
	"context"
	"reflect"
	"testing"
)

func TestWebhookPreferencesPreserveSecretAndURL(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Preferences")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.SetWebhook(ctx, instance.ID, "https://example.com/webhook")
	if err != nil {
		t.Fatal(err)
	}
	selected := []string{"messages", "status"}
	saved, err := store.SaveWebhook(ctx, instance.ID, initial.URL, false, selected, false)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Enabled || saved.Secret != "" || saved.URL != initial.URL {
		t.Fatal("disabling must preserve URL and secret")
	}
	target, err := store.GetWebhookTarget(ctx, instance.ID)
	if err != nil || target.Secret != initial.Secret || target.Allows("message.received") {
		t.Fatal("disabled webhook must not deliver")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config, err := store.GetWebhookConfig(ctx, instance.ID)
	if err != nil || config.Enabled || !reflect.DeepEqual(config.Events, selected) || config.URL != initial.URL {
		t.Fatal("preferences must persist after reopen")
	}
	_, err = store.SaveWebhook(ctx, instance.ID, config.URL, true, config.Events, false)
	if err != nil {
		t.Fatal(err)
	}
	target, err = store.GetWebhookTarget(ctx, instance.ID)
	if err != nil || target.Secret != initial.Secret || !target.Allows("message.receipt") || !target.Allows("status.received") || target.Allows("instance.status") {
		t.Fatal("re-enable must preserve credentials and filter events")
	}
	_, err = store.SaveWebhook(ctx, instance.ID, config.URL, true, []string{}, false)
	if err != nil {
		t.Fatal(err)
	}
	target, _ = store.GetWebhookTarget(ctx, instance.ID)
	if target.Allows("message.sent") {
		t.Fatal("empty selection must send nothing")
	}
	if _, err := store.SaveWebhook(ctx, instance.ID, config.URL, true, []string{"unknown"}, false); err != ErrWebhookEvents {
		t.Fatal("unknown category accepted")
	}
}

func TestWebhookEventCategories(t *testing.T) {
	cases := map[string][]string{
		"messages":   {"message.received", "message.sent", "message.updated", "message.deleted", "message.reaction", "message.receipt"},
		"connection": {"instance.status"}, "status": {"status.received", "status.sent"},
		"presence": {"presence.updated", "presence.chat"}, "groups": {"group.updated"},
	}
	for selected, allowed := range cases {
		target := WebhookTarget{Enabled: true, Events: []string{selected}}
		for category, events := range cases {
			for _, event := range events {
				if target.Allows(event) != (category == selected) {
					t.Fatalf("%s selection mishandled %s", selected, event)
				}
			}
		}
		if len(allowed) == 0 || target.Allows("unknown") {
			t.Fatal("unexpected category")
		}
	}
}
