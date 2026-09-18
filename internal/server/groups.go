package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicListGroups(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.groupInstance(w, r)
	if !ok {
		return
	}
	groups, err := s.groups.ListGroups(r.Context(), instanceID)
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": groups})
}

func (s *Server) publicGetGroup(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.groupInstance(w, r)
	if !ok {
		return
	}
	group, err := s.groups.GetGroup(r.Context(), instanceID, r.PathValue("groupJID"))
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) publicCreateGroup(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.groupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Name         string   `json:"name"`
		Participants []string `json:"participants"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	group, err := s.groups.CreateGroup(r.Context(), instanceID, payload.Name, payload.Participants)
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func (s *Server) publicSetGroupName(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.groupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if s.writeGroupError(w, s.groups.SetGroupName(r.Context(), instanceID, r.PathValue("groupJID"), payload.Name)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicUpdateGroupParticipants(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.groupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Action       string   `json:"action"`
		Participants []string `json:"participants"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	participants, err := s.groups.UpdateGroupParticipants(r.Context(), instanceID, r.PathValue("groupJID"), payload.Action, payload.Participants)
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": participants})
}

func (s *Server) publicGetGroupInvite(w http.ResponseWriter, r *http.Request) {
	s.groupInvite(w, r, false)
}

func (s *Server) publicRotateGroupInvite(w http.ResponseWriter, r *http.Request) {
	s.groupInvite(w, r, true)
}

func (s *Server) groupInvite(w http.ResponseWriter, r *http.Request, reset bool) {
	instanceID, ok := s.groupInstance(w, r)
	if !ok {
		return
	}
	link, err := s.groups.GroupInviteLink(r.Context(), instanceID, r.PathValue("groupJID"), reset)
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"link": link})
}

func (s *Server) publicJoinGroup(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.groupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	jid, err := s.groups.JoinGroup(r.Context(), instanceID, payload.Code)
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"jid": jid})
}

func (s *Server) publicSetGroupDescription(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Description string `json:"description"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if s.writeGroupError(w, s.advancedGroups.SetGroupDescription(r.Context(), instanceID, r.PathValue("groupJID"), payload.Description)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicSetGroupPhoto(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, engine.MaxProfilePhotoBytes+mediaRequestOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) || strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("photo must have at most %d MB", engine.MaxProfilePhotoBytes>>20))
		} else {
			writeError(w, http.StatusBadRequest, "request must use multipart/form-data")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "photo file is required")
		return
	}
	defer file.Close()
	photo, err := io.ReadAll(io.LimitReader(file, engine.MaxProfilePhotoBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read photo")
		return
	}
	if len(photo) > engine.MaxProfilePhotoBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("photo must have at most %d MB", engine.MaxProfilePhotoBytes>>20))
		return
	}
	pictureID, err := s.advancedGroups.SetGroupPhoto(r.Context(), instanceID, r.PathValue("groupJID"), photo)
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pictureId": pictureID})
}

func (s *Server) publicDeleteGroupPhoto(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	_, err := s.advancedGroups.SetGroupPhoto(r.Context(), instanceID, r.PathValue("groupJID"), nil)
	if s.writeGroupError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicLeaveGroup(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	if s.writeGroupError(w, s.advancedGroups.LeaveGroup(r.Context(), instanceID, r.PathValue("groupJID"))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicSetGroupPermissions(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Announce *bool `json:"announce,omitempty"`
		Locked   *bool `json:"locked,omitempty"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if s.writeGroupError(w, s.advancedGroups.SetGroupPermissions(r.Context(), instanceID, r.PathValue("groupJID"), payload.Announce, payload.Locked)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicSetGroupJoinApproval(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &payload); err != nil || payload.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	if s.writeGroupError(w, s.advancedGroups.SetGroupJoinApproval(r.Context(), instanceID, r.PathValue("groupJID"), *payload.Enabled)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicListGroupJoinRequests(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	requests, err := s.advancedGroups.ListGroupJoinRequests(r.Context(), instanceID, r.PathValue("groupJID"))
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": requests})
}

func (s *Server) publicUpdateGroupJoinRequests(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.advancedGroupInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Action       string   `json:"action"`
		Participants []string `json:"participants"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	participants, err := s.advancedGroups.UpdateGroupJoinRequests(r.Context(), instanceID, r.PathValue("groupJID"), payload.Action, payload.Participants)
	if s.writeGroupError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": participants})
}

func (s *Server) groupInstance(w http.ResponseWriter, r *http.Request) (string, bool) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return "", false
	}
	if s.groups == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return "", false
	}
	return instanceID, true
}

func (s *Server) advancedGroupInstance(w http.ResponseWriter, r *http.Request) (string, bool) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return "", false
	}
	if s.advancedGroups == nil {
		writeError(w, http.StatusServiceUnavailable, "advanced WhatsApp group engine is unavailable")
		return "", false
	}
	return instanceID, true
}

func (s *Server) writeGroupError(w http.ResponseWriter, err error) bool {
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
