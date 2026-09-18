package server

import (
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicSendLocation(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Recipient string               `json:"recipient"`
		Options   *sendOptions         `json:"options,omitempty"`
		ReplyTo   *engine.ReplyOptions `json:"replyTo,omitempty"`
		Mentions  []string             `json:"mentions,omitempty"`
		Forwarded bool                 `json:"forwarded,omitempty"`
		engine.LocationPayload
	}
	s.sendStructuredJSON(w, r, &payload, &payload.Recipient, &payload.Options, func(instanceID string) (engine.SentMessage, error) {
		if err := engine.ValidateLocation(&payload.LocationPayload); err != nil {
			return engine.SentMessage{}, err
		}
		if advanced, supported := s.structured.(AdvancedStructuredMessageSender); supported {
			return advanced.SendLocationAdvanced(r.Context(), instanceID, payload.Recipient, payload.LocationPayload, engine.MessageOptions{ReplyTo: payload.ReplyTo, Mentions: payload.Mentions, Forwarded: payload.Forwarded})
		}
		if payload.ReplyTo != nil || len(payload.Mentions) > 0 || payload.Forwarded {
			return engine.SentMessage{}, errors.New("advanced message options are unavailable")
		}
		return s.structured.SendLocation(r.Context(), instanceID, payload.Recipient, payload.LocationPayload)
	})
}

func (s *Server) publicSendLiveLocation(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Recipient string               `json:"recipient"`
		Options   *sendOptions         `json:"options,omitempty"`
		ReplyTo   *engine.ReplyOptions `json:"replyTo,omitempty"`
		Mentions  []string             `json:"mentions,omitempty"`
		Forwarded bool                 `json:"forwarded,omitempty"`
		engine.LiveLocationPayload
	}
	s.sendStructuredJSON(w, r, &payload, &payload.Recipient, &payload.Options, func(instanceID string) (engine.SentMessage, error) {
		if err := engine.ValidateLiveLocation(&payload.LiveLocationPayload); err != nil {
			return engine.SentMessage{}, err
		}
		sender, supported := s.structured.(LiveLocationSender)
		if !supported {
			return engine.SentMessage{}, errors.New("live location is not supported by the configured engine")
		}
		w.Header().Set("X-Wirely-Experimental", "true")
		return sender.SendLiveLocation(r.Context(), instanceID, payload.Recipient, payload.LiveLocationPayload, engine.MessageOptions{ReplyTo: payload.ReplyTo, Mentions: payload.Mentions, Forwarded: payload.Forwarded})
	})
}

func (s *Server) publicSendContact(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Recipient string               `json:"recipient"`
		Options   *sendOptions         `json:"options,omitempty"`
		ReplyTo   *engine.ReplyOptions `json:"replyTo,omitempty"`
		Mentions  []string             `json:"mentions,omitempty"`
		Forwarded bool                 `json:"forwarded,omitempty"`
		engine.ContactPayload
	}
	s.sendStructuredJSON(w, r, &payload, &payload.Recipient, &payload.Options, func(instanceID string) (engine.SentMessage, error) {
		if err := engine.ValidateContact(&payload.ContactPayload); err != nil {
			return engine.SentMessage{}, err
		}
		if advanced, supported := s.structured.(AdvancedStructuredMessageSender); supported {
			return advanced.SendContactAdvanced(r.Context(), instanceID, payload.Recipient, payload.ContactPayload, engine.MessageOptions{ReplyTo: payload.ReplyTo, Mentions: payload.Mentions, Forwarded: payload.Forwarded})
		}
		if payload.ReplyTo != nil || len(payload.Mentions) > 0 || payload.Forwarded {
			return engine.SentMessage{}, errors.New("advanced message options are unavailable")
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
