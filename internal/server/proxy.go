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
	URL                  string `json:"url"`
	AutoReconnect        bool   `json:"autoReconnect"`
	RetryIntervalSeconds int    `json:"retryIntervalSeconds"`
	ReconnectAttempts    int    `json:"reconnectAttempts"`
}

type proxyResponse struct {
	Configured           bool   `json:"configured"`
	URL                  string `json:"url,omitempty"`
	Scheme               string `json:"scheme,omitempty"`
	Host                 string `json:"host,omitempty"`
	Port                 string `json:"port,omitempty"`
	Username             string `json:"username,omitempty"`
	HasPassword          bool   `json:"hasPassword"`
	AutoReconnect        bool   `json:"autoReconnect"`
	RetryIntervalSeconds int    `json:"retryIntervalSeconds"`
	ReconnectAttempts    int    `json:"reconnectAttempts"`
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

func describeProxy(settings storage.ProxySettings) proxyResponse {
	response := proxyResponse{
		AutoReconnect: settings.AutoReconnect, RetryIntervalSeconds: settings.RetryIntervalSeconds,
		ReconnectAttempts: settings.ReconnectAttempts,
	}
	if settings.URL == "" {
		return response
	}
	parsed, _ := url.Parse(settings.URL)
	username := ""
	hasPassword := false
	if parsed.User != nil {
		username = parsed.User.Username()
		_, hasPassword = parsed.User.Password()
		if hasPassword {
			parsed.User = url.UserPassword(username, "********")
		}
	}
	response.Configured = true
	response.URL, response.Scheme, response.Host, response.Port = parsed.String(), parsed.Scheme, parsed.Hostname(), parsed.Port()
	response.Username, response.HasPassword = username, hasPassword
	return response
}

func validateProxyReconnect(request proxyRequest) error {
	if request.RetryIntervalSeconds < storage.DefaultProxyRetryInterval || request.RetryIntervalSeconds > 86400 {
		return errors.New("proxy retry interval must be between 60 and 86400 seconds")
	}
	if request.ReconnectAttempts < 0 || request.ReconnectAttempts > 10000 {
		return errors.New("proxy reconnect attempts must be between 0 and 10000")
	}
	return nil
}

func (s *Server) saveInstanceProxy(ctx context.Context, id string, request proxyRequest) (proxyResponse, error) {
	current, err := s.store.GetInstanceProxySettings(ctx, id)
	if err != nil {
		return proxyResponse{}, err
	}
	providedURL := strings.TrimSpace(request.URL)
	value := providedURL
	if value == "" {
		value = current.URL
	}
	validated, err := validateProxyURL(value)
	if err != nil {
		return proxyResponse{}, err
	}
	if request.RetryIntervalSeconds == 0 {
		request.RetryIntervalSeconds = storage.DefaultProxyRetryInterval
	}
	if err = validateProxyReconnect(request); err != nil {
		return proxyResponse{}, err
	}
	settings := storage.ProxySettings{URL: validated, AutoReconnect: request.AutoReconnect,
		RetryIntervalSeconds: request.RetryIntervalSeconds, ReconnectAttempts: request.ReconnectAttempts}
	if err = s.store.SaveInstanceProxySettings(ctx, id, settings); err != nil {
		return proxyResponse{}, err
	}
	if s.engine != nil {
		if err = s.engine.SetProxySettings(id, settings, providedURL != ""); err != nil {
			return proxyResponse{}, err
		}
	}
	return describeProxy(settings), nil
}

func (s *Server) clearInstanceProxy(ctx context.Context, id string) error {
	settings := storage.ProxySettings{RetryIntervalSeconds: storage.DefaultProxyRetryInterval}
	if err := s.store.SaveInstanceProxySettings(ctx, id, settings); err != nil {
		return err
	}
	if s.engine != nil {
		return s.engine.ClearProxy(id)
	}
	return nil
}

func (s *Server) getInstanceProxy(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetInstanceProxySettings(r.Context(), r.PathValue("id"))
	if errors.Is(err, storage.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load proxy")
		return
	}
	writeJSON(w, http.StatusOK, describeProxy(settings))
}

func (s *Server) setInstanceProxy(w http.ResponseWriter, r *http.Request) {
	var payload proxyRequest
	if decodeJSON(w, r, &payload) != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := s.saveInstanceProxy(r.Context(), r.PathValue("id"), payload)
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
