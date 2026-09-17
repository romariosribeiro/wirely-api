package server

import (
	"errors"
	"net/http"

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
