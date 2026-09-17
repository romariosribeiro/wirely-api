package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) listContacts(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	if s.contacts == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp contact store is unavailable")
		return
	}
	page, pageSize, ok := pagination(w, r)
	if !ok {
		return
	}
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	if len([]rune(search)) > 100 {
		writeError(w, http.StatusBadRequest, "search must have at most 100 characters")
		return
	}
	contacts, err := s.contacts.Contacts(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "failed to load WhatsApp contacts")
		return
	}
	filtered := contacts[:0]
	for _, contact := range contacts {
		haystack := strings.ToLower(contact.Name + " " + contact.Phone + " " + contact.JID + " " + contact.BusinessName)
		if search == "" || strings.Contains(haystack, search) {
			filtered = append(filtered, contact)
		}
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, pageResponse(filtered[start:end], page, pageSize, total))
}

func (s *Server) listChats(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	page, pageSize, ok := pagination(w, r)
	if !ok {
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len([]rune(search)) > 100 {
		writeError(w, http.StatusBadRequest, "search must have at most 100 characters")
		return
	}
	names := make(map[string]string)
	includedChats := make([]string, 0)
	if s.contacts != nil {
		if contacts, contactErr := s.contacts.Contacts(r.Context(), instanceID); contactErr == nil {
			query := strings.ToLower(search)
			for _, contact := range contacts {
				names[contact.JID] = contact.Name
				haystack := strings.ToLower(contact.Name + " " + contact.Phone + " " + contact.JID + " " + contact.BusinessName)
				if query != "" && strings.Contains(haystack, query) {
					includedChats = append(includedChats, contact.JID)
				}
			}
		}
	}
	items, total, err := s.store.ListChatsIncluding(r.Context(), instanceID, search, includedChats, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list chats")
		return
	}
	for index := range items {
		if name := names[items[index].Chat]; name != "" {
			items[index].Name = name
		}
	}
	writeJSON(w, http.StatusOK, pageResponse(items, page, pageSize, total))
}

func (s *Server) listChatMessages(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	chat, ok := chatPathValue(w, r)
	if !ok {
		return
	}
	page, pageSize, ok := pagination(w, r)
	if !ok {
		return
	}
	items, total, err := s.store.ListChatMessages(r.Context(), instanceID, chat, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list chat messages")
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(items, page, pageSize, total))
}

func (s *Server) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	chat, ok := chatPathValue(w, r)
	if !ok {
		return
	}
	if s.chatSender == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	var payload struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := s.chatSender.SendChatText(r.Context(), instanceID, chat, payload.Message)
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

func (s *Server) markChatRead(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	chat, ok := chatPathValue(w, r)
	if !ok {
		return
	}
	if err := s.store.MarkChatRead(r.Context(), instanceID, chat); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark chat as read")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func chatPathValue(w http.ResponseWriter, r *http.Request) (string, bool) {
	chat := strings.TrimSpace(r.PathValue("chat"))
	if chat == "" || len(chat) > 200 || !strings.Contains(chat, "@") {
		writeError(w, http.StatusBadRequest, "invalid chat identifier")
		return "", false
	}
	return chat, true
}
