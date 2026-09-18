package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

type proxyRequest struct {
	URL string `json:"url"`
}

type proxyResponse struct {
	Configured  bool   `json:"configured"`
	URL         string `json:"url,omitempty"`
	Scheme      string `json:"scheme,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        string `json:"port,omitempty"`
	Username    string `json:"username,omitempty"`
	HasPassword bool   `json:"hasPassword"`
}

func validateProxyURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
		return "", errors.New("proxy URL must include host and port")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5":
	default:
		return "", errors.New("proxy scheme must be http, https, or socks5")
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("proxy URL cannot include path, query, or fragment")
	}
	return parsed.String(), nil
}

func describeProxy(value string) proxyResponse {
	if value == "" {
		return proxyResponse{}
	}
	parsed, _ := url.Parse(value)
	username := ""
	hasPassword := false
	if parsed.User != nil {
		username = parsed.User.Username()
		_, hasPassword = parsed.User.Password()
		if hasPassword {
			parsed.User = url.UserPassword(username, "********")
		}
	}
	return proxyResponse{Configured: true, URL: parsed.String(), Scheme: parsed.Scheme, Host: parsed.Hostname(), Port: parsed.Port(), Username: username, HasPassword: hasPassword}
}

func (s *Server) saveInstanceProxy(ctx context.Context, id, value string) (proxyResponse, error) {
	validated, err := validateProxyURL(value)
	if err != nil {
		return proxyResponse{}, err
	}
	if err = s.store.SaveInstanceProxy(ctx, id, validated); err != nil {
		return proxyResponse{}, err
	}
	if s.engine != nil {
		if err = s.engine.SetProxy(id, validated); err != nil {
			return proxyResponse{}, err
		}
	}
	return describeProxy(validated), nil
}

func (s *Server) clearInstanceProxy(ctx context.Context, id string) error {
	if err := s.store.SaveInstanceProxy(ctx, id, ""); err != nil {
		return err
	}
	if s.engine != nil {
		return s.engine.ClearProxy(id)
	}
	return nil
}

func (s *Server) getInstanceProxy(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.GetInstanceProxy(r.Context(), r.PathValue("id"))
	if errors.Is(err, storage.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load proxy")
		return
	}
	writeJSON(w, http.StatusOK, describeProxy(value))
}

func (s *Server) setInstanceProxy(w http.ResponseWriter, r *http.Request) {
	var payload proxyRequest
	if decodeJSON(w, r, &payload) != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := s.saveInstanceProxy(r.Context(), r.PathValue("id"), payload.URL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) deleteInstanceProxy(w http.ResponseWriter, r *http.Request) {
	if err := s.clearInstanceProxy(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
