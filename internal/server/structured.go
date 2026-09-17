package server

import (
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicSendLocation(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Recipient string       `json:"recipient"`
		Options   *sendOptions `json:"options,omitempty"`
		engine.LocationPayload
	}
	s.sendStructuredJSON(w, r, &payload, &payload.Recipient, &payload.Options, func(instanceID string) (engine.SentMessage, error) {
		if err := engine.ValidateLocation(&payload.LocationPayload); err != nil {
			return engine.SentMessage{}, err
		}
		return s.structured.SendLocation(r.Context(), instanceID, payload.Recipient, payload.LocationPayload)
	})
}

func (s *Server) publicSendContact(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Recipient string       `json:"recipient"`
		Options   *sendOptions `json:"options,omitempty"`
		engine.ContactPayload
	}
	s.sendStructuredJSON(w, r, &payload, &payload.Recipient, &payload.Options, func(instanceID string) (engine.SentMessage, error) {
		if err := engine.ValidateContact(&payload.ContactPayload); err != nil {
			return engine.SentMessage{}, err
		}
		return s.structured.SendContact(r.Context(), instanceID, payload.Recipient, payload.ContactPayload)
	})
}

func (s *Server) publicSendPoll(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Recipient string       `json:"recipient"`
		Question  string       `json:"question"`
		Choices   []string     `json:"choices"`
		MaxAnswer int          `json:"maxAnswer"`
		Options   *sendOptions `json:"options,omitempty"`
	}
	s.sendStructuredJSON(w, r, &payload, &payload.Recipient, &payload.Options, func(instanceID string) (engine.SentMessage, error) {
		poll := engine.PollPayload{Question: payload.Question, Options: payload.Choices, MaxAnswer: payload.MaxAnswer}
		if err := engine.ValidatePoll(&poll); err != nil {
			return engine.SentMessage{}, err
		}
		return s.structured.SendPoll(r.Context(), instanceID, payload.Recipient, poll)
	})
}

func (s *Server) publicSendReaction(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Recipient string       `json:"recipient"`
		Options   *sendOptions `json:"options,omitempty"`
		engine.ReactionPayload
	}
	s.sendStructuredJSON(w, r, &payload, &payload.Recipient, &payload.Options, func(instanceID string) (engine.SentMessage, error) {
		if err := engine.ValidateReaction(&payload.ReactionPayload); err != nil {
			return engine.SentMessage{}, err
		}
		return s.structured.SendReaction(r.Context(), instanceID, payload.Recipient, payload.ReactionPayload)
	})
}

func (s *Server) sendStructuredJSON(w http.ResponseWriter, r *http.Request, payload any, recipient *string, options **sendOptions, send func(string) (engine.SentMessage, error)) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.structured == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := decodeJSON(w, r, payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	stopPresence, ok := s.beginSendOptions(w, r, instanceID, *recipient, *options)
	if !ok {
		return
	}
	defer stopPresence()
	result, err := send(instanceID)
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
