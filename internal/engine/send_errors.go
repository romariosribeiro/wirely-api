package engine

import "go.mau.fi/whatsmeow"

// ErrRecipientNotOnWhatsApp also matches wrapped errors returned by the engine.
var ErrRecipientNotOnWhatsApp = whatsmeow.ErrRecipientNotOnWhatsApp

const RecipientNotOnWhatsAppMessage = "recipient is not on WhatsApp; check the phone number and country code before sending again"
