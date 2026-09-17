package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func (s *Server) publicDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Chat        string `json:"chat"`
		MessageID   string `json:"messageId"`
		Participant string `json:"participant,omitempty"`
	}
	s.handleMessageAction(w, r, &payload, func(instanceID string) (engine.MessageActionResult, error) {
		return s.messageActions.DeleteMessage(r.Context(), instanceID, payload.Chat, payload.MessageID, payload.Participant)
	})
}

func (s *Server) publicEditMessage(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Chat      string `json:"chat"`
		MessageID string `json:"messageId"`
		Message   string `json:"message"`
	}
	s.handleMessageAction(w, r, &payload, func(instanceID string) (engine.MessageActionResult, error) {
		return s.messageActions.EditMessage(r.Context(), instanceID, payload.Chat, payload.MessageID, payload.Message)
	})
}

func (s *Server) publicMarkMessagesRead(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Chat        string   `json:"chat"`
		MessageIDs  []string `json:"messageIds"`
		Participant string   `json:"participant,omitempty"`
	}
	s.handleMessageAction(w, r, &payload, func(instanceID string) (engine.MessageActionResult, error) {
		return s.messageActions.MarkMessagesRead(r.Context(), instanceID, payload.Chat, payload.MessageIDs, payload.Participant)
	})
}

func (s *Server) handleMessageAction(w http.ResponseWriter, r *http.Request, payload any, action func(string) (engine.MessageActionResult, error)) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.messageActions == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := decodeJSON(w, r, payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := action(instanceID)
	if writeMessageActionError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) publicGetMessageStatus(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	if messageID == "" || len(messageID) > 200 {
		writeError(w, http.StatusUnprocessableEntity, "messageId is invalid")
		return
	}
	status, err := s.store.GetMessageStatus(r.Context(), instanceID, messageID)
	if errors.Is(err, storage.ErrMessageStatusMissing) {
		writeError(w, http.StatusNotFound, "message status not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load message status")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) publicArchiveChat(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Chat     string `json:"chat"`
		Archived *bool  `json:"archived,omitempty"`
	}
	s.handleChatAction(w, r, &payload, func(instanceID string) (engine.ChatActionResult, error) {
		archived := true
		if payload.Archived != nil {
			archived = *payload.Archived
		}
		return s.messageActions.ArchiveChat(r.Context(), instanceID, payload.Chat, archived)
	})
}

func (s *Server) publicMuteChat(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Chat            string `json:"chat"`
		DurationSeconds int64  `json:"durationSeconds,omitempty"`
	}
	s.handleChatAction(w, r, &payload, func(instanceID string) (engine.ChatActionResult, error) {
		if payload.DurationSeconds < 0 || payload.DurationSeconds > int64((365*24*time.Hour)/time.Second) {
			return engine.ChatActionResult{}, errors.New("durationSeconds must be between 0 and 31536000")
		}
		return s.messageActions.MuteChat(r.Context(), instanceID, payload.Chat, time.Duration(payload.DurationSeconds)*time.Second)
	})
}

func (s *Server) publicPinChat(w http.ResponseWriter, r *http.Request) {
	s.publicSetChatPin(w, r, true)
}

func (s *Server) publicUnpinChat(w http.ResponseWriter, r *http.Request) {
	s.publicSetChatPin(w, r, false)
}

func (s *Server) publicSetChatPin(w http.ResponseWriter, r *http.Request, pinned bool) {
	var payload struct {
		Chat string `json:"chat"`
	}
	s.handleChatAction(w, r, &payload, func(instanceID string) (engine.ChatActionResult, error) {
		return s.messageActions.PinChat(r.Context(), instanceID, payload.Chat, pinned)
	})
}

func (s *Server) handleChatAction(w http.ResponseWriter, r *http.Request, payload any, action func(string) (engine.ChatActionResult, error)) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.messageActions == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := decodeJSON(w, r, payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := action(instanceID)
	if writeMessageActionError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeMessageActionError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
	} else {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
	return true
}
