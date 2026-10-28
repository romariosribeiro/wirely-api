package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

type fakeRecipientLookup struct {
	response []types.IsOnWhatsAppResponse
	err      error
	queries  []string
}

func (f *fakeRecipientLookup) IsOnWhatsApp(_ context.Context, queries []string) ([]types.IsOnWhatsAppResponse, error) {
	f.queries = queries
	return f.response, f.err
}

func TestResolveMessageRecipientRegistration(t *testing.T) {
	withNine := types.NewJID("5548988150709", types.DefaultUserServer)
	withoutNine := types.NewJID("554888150709", types.DefaultUserServer)
	negative := []types.IsOnWhatsAppResponse{{Query: withNine.User}, {Query: withoutNine.User}}
	for _, test := range []struct {
		name         string
		response     []types.IsOnWhatsAppResponse
		lookupErr    error
		want         types.JID
		unregistered bool
	}{
		{name: "confirmed absent", response: negative, unregistered: true},
		{name: "partial response", response: negative[:1], want: withNine},
		{name: "empty response", want: withNine},
		{name: "query failure", response: negative, lookupErr: context.DeadlineExceeded, want: withNine},
		{name: "ninth digit fallback", response: []types.IsOnWhatsAppResponse{{Query: withNine.User}, {Query: withoutNine.User, IsIn: true, JID: withoutNine}}, want: withoutNine},
		{name: "incomplete registered result", response: []types.IsOnWhatsAppResponse{{Query: withNine.User, IsIn: true}, {Query: withoutNine.User}}, want: withNine},
		{name: "unrelated results", response: []types.IsOnWhatsAppResponse{{Query: "5511000000000"}}, want: withNine},
	} {
		t.Run(test.name, func(t *testing.T) {
			lookup := &fakeRecipientLookup{response: test.response, err: test.lookupErr}
			jid, _, err := resolveMessageRecipient(context.Background(), lookup, withNine.User)
			if errors.Is(err, ErrRecipientNotOnWhatsApp) != test.unregistered || (err != nil && !test.unregistered) || jid != test.want {
				t.Fatalf("got jid=%s error=%v; want jid=%s unregistered=%v", jid, err, test.want, test.unregistered)
			}
			if !reflect.DeepEqual(lookup.queries, []string{withNine.User, withoutNine.User}) {
				t.Fatalf("unexpected queries: %v", lookup.queries)
			}
		})
	}
}

func TestResolveMessageRecipientSkipsVariantLookup(t *testing.T) {
	for _, recipient := range []string{"351912345678", "120363000000000000@g.us", "1234567890@lid", "5548988150709@s.whatsapp.net"} {
		lookup := &fakeRecipientLookup{err: errors.New("must not query")}
		if _, _, err := resolveMessageRecipient(context.Background(), lookup, recipient); err != nil || len(lookup.queries) != 0 {
			t.Fatalf("%s: error=%v queries=%v", recipient, err, lookup.queries)
		}
	}
}

func TestBrazilianPhoneVariants(t *testing.T) {
	tests := []struct {
		phone string
		want  []string
	}{
		{phone: "5548988150709", want: []string{"5548988150709", "554888150709"}},
		{phone: "554888150709", want: []string{"554888150709", "5548988150709"}},
		{phone: "351912345678", want: []string{"351912345678"}},
	}
	for _, test := range tests {
		if got := brazilianPhoneVariants(test.phone); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("brazilianPhoneVariants(%q) = %#v, want %#v", test.phone, got, test.want)
		}
	}
}

func TestSelectCanonicalRecipientPrefersRequestedVariant(t *testing.T) {
	withNine := types.NewJID("5548988150709", types.DefaultUserServer)
	withoutNine := types.NewJID("554888150709", types.DefaultUserServer)
	response := []types.IsOnWhatsAppResponse{
		{Query: "554888150709", JID: withoutNine, IsIn: true},
		{Query: "5548988150709", JID: withNine, IsIn: true},
	}

	got, ok := selectCanonicalRecipient([]string{"5548988150709", "554888150709"}, response)
	if !ok || got != withNine {
		t.Fatalf("selected %s, %v; want %s", got, ok, withNine)
	}

	response[1].IsIn = false
	got, ok = selectCanonicalRecipient([]string{"5548988150709", "554888150709"}, response)
	if !ok || got != withoutNine {
		t.Fatalf("fallback selected %s, %v; want %s", got, ok, withoutNine)
	}
}
