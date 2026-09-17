package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

const maxSendDelayMS = 60_000

type sendOptions struct {
	Presence string `json:"presence"`
	Delay    int    `json:"delay"`
}

func parseMultipartSendOptions(value string) (*sendOptions, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var options sendOptions
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return nil, errors.New("options must be valid JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("options must contain one JSON object")
	}
	return &options, nil
}

func validateSendOptions(options *sendOptions) error {
	if options == nil {
		return nil
	}
	options.Presence = strings.ToLower(strings.TrimSpace(options.Presence))
	if options.Presence != "composing" && options.Presence != "recording" {
		return errors.New("options.presence must be composing or recording")
	}
	if options.Delay < 0 || options.Delay > maxSendDelayMS {
		return fmt.Errorf("options.delay must be between 0 and %d milliseconds", maxSendDelayMS)
	}
	return nil
}

func waitSendDelay(ctx context.Context, delay int) error {
	if delay == 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) beginSendOptions(w http.ResponseWriter, r *http.Request, instanceID, recipient string, options *sendOptions) (func(), bool) {
	noop := func() {}
	if err := validateSendOptions(options); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return noop, false
	}
	if options == nil {
		return noop, true
	}
	if s.presence == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp presence engine is unavailable")
		return noop, false
	}
	if err := s.presence.SendChatPresence(r.Context(), instanceID, recipient, options.Presence); err != nil {
		writeSendOptionsError(w, err)
		return noop, false
	}
	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.presence.SendChatPresence(ctx, instanceID, recipient, "paused")
	}
	if err := waitSendDelay(r.Context(), options.Delay); err != nil {
		stop()
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		}
		return noop, false
	}
	return stop, true
}

func writeSendOptionsError(w http.ResponseWriter, err error) {
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusUnprocessableEntity, err.Error())
}
