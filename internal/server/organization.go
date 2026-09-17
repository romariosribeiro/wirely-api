package server

import (
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicCreateNewsletter(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	item, err := s.organization.CreateNewsletter(r.Context(), instanceID, payload.Name, payload.Description)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) publicGetNewsletter(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		JID string `json:"jid"`
	}
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	item, err := s.organization.GetNewsletter(r.Context(), instanceID, payload.JID)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) publicGetNewsletterByInvite(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Key string `json:"key"`
	}
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	item, err := s.organization.GetNewsletterByInvite(r.Context(), instanceID, payload.Key)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) publicListNewsletters(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	items, err := s.organization.ListNewsletters(r.Context(), instanceID)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) publicGetNewsletterMessages(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		JID    string `json:"jid"`
		Count  int    `json:"count,omitempty"`
		Before int64  `json:"before,omitempty"`
	}
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	items, err := s.organization.GetNewsletterMessages(r.Context(), instanceID, payload.JID, payload.Count, payload.Before)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) publicSubscribeNewsletter(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		JID string `json:"jid"`
	}
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	if s.writeOrganizationError(w, s.organization.SubscribeNewsletter(r.Context(), instanceID, payload.JID)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jid": payload.JID, "subscribed": true})
}

type labelAssociationPayload struct {
	Chat      string `json:"chat"`
	LabelID   string `json:"labelId"`
	MessageID string `json:"messageId,omitempty"`
}

func (s *Server) publicAddChatLabel(w http.ResponseWriter, r *http.Request) {
	s.publicSetChatLabel(w, r, true)
}
func (s *Server) publicRemoveChatLabel(w http.ResponseWriter, r *http.Request) {
	s.publicSetChatLabel(w, r, false)
}
func (s *Server) publicSetChatLabel(w http.ResponseWriter, r *http.Request, labeled bool) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload labelAssociationPayload
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	result, err := s.organization.SetChatLabel(r.Context(), instanceID, payload.Chat, payload.LabelID, labeled)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) publicAddMessageLabel(w http.ResponseWriter, r *http.Request) {
	s.publicSetMessageLabel(w, r, true)
}
func (s *Server) publicRemoveMessageLabel(w http.ResponseWriter, r *http.Request) {
	s.publicSetMessageLabel(w, r, false)
}
func (s *Server) publicSetMessageLabel(w http.ResponseWriter, r *http.Request, labeled bool) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload labelAssociationPayload
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	result, err := s.organization.SetMessageLabel(r.Context(), instanceID, payload.Chat, payload.MessageID, payload.LabelID, labeled)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) publicEditLabel(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Name    string `json:"name"`
		Color   int32  `json:"color"`
		Deleted bool   `json:"deleted,omitempty"`
	}
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	result, err := s.organization.EditLabel(r.Context(), instanceID, r.PathValue("labelID"), payload.Name, payload.Color, payload.Deleted)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) publicCreateCommunity(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Name string `json:"name"`
	}
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	result, err := s.organization.CreateCommunity(r.Context(), instanceID, payload.Name)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

type communityGroupsPayload struct {
	CommunityJID string   `json:"communityJid"`
	GroupJIDs    []string `json:"groupJids"`
}

func (s *Server) publicAddCommunityGroups(w http.ResponseWriter, r *http.Request) {
	s.publicUpdateCommunityGroups(w, r, true)
}
func (s *Server) publicRemoveCommunityGroups(w http.ResponseWriter, r *http.Request) {
	s.publicUpdateCommunityGroups(w, r, false)
}
func (s *Server) publicUpdateCommunityGroups(w http.ResponseWriter, r *http.Request, link bool) {
	instanceID, ok := s.organizationInstance(w, r)
	if !ok {
		return
	}
	var payload communityGroupsPayload
	if !decodeOrganizationJSON(w, r, &payload) {
		return
	}
	result, err := s.organization.UpdateCommunityGroups(r.Context(), instanceID, payload.CommunityJID, payload.GroupJIDs, link)
	if s.writeOrganizationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) organizationInstance(w http.ResponseWriter, r *http.Request) (string, bool) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return "", false
	}
	if s.organization == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return "", false
	}
	return instanceID, true
}

func decodeOrganizationJSON(w http.ResponseWriter, r *http.Request, payload any) bool {
	if err := decodeJSON(w, r, payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return false
	}
	return true
}

func (s *Server) writeOrganizationError(w http.ResponseWriter, err error) bool {
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
