package engine

import (
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

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
