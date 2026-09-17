package storage

import (
	"context"
	"reflect"
	"testing"
)

func TestInstanceSettingsLifecycle(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	initial := InstanceSettings{
		AlwaysOnline: true, RejectCall: true, MsgRejectCall: "Estou indisponível",
		ReadMessages: true, IgnoreGroups: true,
	}
	instance, err := store.CreateInstanceWithSettings(context.Background(), "Automação", initial)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(instance.InstanceSettings, initial) {
		t.Fatalf("created settings = %#v, want %#v", instance.InstanceSettings, initial)
	}

	updated := InstanceSettings{RejectCall: true, MsgRejectCall: "Envie uma mensagem", IgnoreStatus: true}
	if err := store.UpdateInstanceSettings(context.Background(), instance.ID, updated); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetInstanceSettings(context.Background(), instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, updated) {
		t.Fatalf("loaded settings = %#v, want %#v", loaded, updated)
	}

	instances, err := store.ListInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || !reflect.DeepEqual(instances[0].InstanceSettings, updated) {
		t.Fatalf("listed settings = %#v, want %#v", instances, updated)
	}
}
