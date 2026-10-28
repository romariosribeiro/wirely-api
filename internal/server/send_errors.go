package server

import (
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

// Keep the existing error string contract; structured fields are additive.
func writeSendError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, engine.ErrRecipientNotOnWhatsApp):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":     engine.RecipientNotOnWhatsAppMessage,
			"code":      "recipient_not_on_whatsapp",
			"retryable": false,
		})
	case errors.Is(err, engine.ErrNotConnected):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
	return true
}
