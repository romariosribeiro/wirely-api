package engine

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

type recipientLookup interface {
	IsOnWhatsApp(context.Context, []string) ([]types.IsOnWhatsAppResponse, error)
}

// resolveMessageRecipient asks WhatsApp for the canonical JID when a Brazilian
// phone number may be registered with or without the ninth mobile digit.
func resolveMessageRecipient(ctx context.Context, client recipientLookup, value string) (types.JID, string, error) {
	jid, display, err := parseMessageRecipient(value)
	if err != nil {
		return types.EmptyJID, "", err
	}
	if strings.Contains(strings.TrimSpace(value), "@") || jid.Server != types.DefaultUserServer {
		return jid, display, nil
	}

	variants := brazilianPhoneVariants(jid.User)
	if len(variants) < 2 {
		return jid, display, nil
	}
	response, err := client.IsOnWhatsApp(ctx, variants)
	if err != nil {
		// Contact lookup must not make sending unavailable during a transient
		// WhatsApp query failure. The regular send path can still resolve it.
		return jid, display, nil
	}
	if canonical, ok := selectCanonicalRecipient(variants, response); ok {
		return canonical, display, nil
	}
	// Only reject a lookup when WhatsApp explicitly rejected every variant.
	// Partial responses or query failures must not be mistaken for an absent account.
	if allRecipientsUnregistered(variants, response) {
		return types.EmptyJID, "", ErrRecipientNotOnWhatsApp
	}
	return jid, display, nil
}

func allRecipientsUnregistered(variants []string, response []types.IsOnWhatsAppResponse) bool {
	for _, variant := range variants {
		confirmed := false
		for _, item := range response {
			if item.Query == variant {
				if item.IsIn {
					return false
				}
				confirmed = true
			}
		}
		if !confirmed {
			return false
		}
	}
	return len(variants) > 0
}

func brazilianPhoneVariants(phone string) []string {
	if !strings.HasPrefix(phone, "55") {
		return []string{phone}
	}
	switch {
	case len(phone) == 13 && phone[4] == '9':
		return []string{phone, phone[:4] + phone[5:]}
	case len(phone) == 12:
		return []string{phone, phone[:4] + "9" + phone[4:]}
	default:
		return []string{phone}
	}
}

func selectCanonicalRecipient(variants []string, response []types.IsOnWhatsAppResponse) (types.JID, bool) {
	for _, variant := range variants {
		for _, item := range response {
			if item.Query != variant || !item.IsIn {
				continue
			}
			jid := item.JID
			if jid.IsEmpty() {
				jid = item.PhoneNumber
			}
			if !jid.IsEmpty() {
				return jid.ToNonAD(), true
			}
		}
	}
	return types.EmptyJID, false
}
