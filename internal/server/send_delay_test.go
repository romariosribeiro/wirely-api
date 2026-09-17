package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingPresenceSender struct {
	states []string
}

func (s *recordingPresenceSender) SendChatPresence(_ context.Context, _, _, presence string) error {
	s.states = append(s.states, presence)
	return nil
}

func TestParseAndValidateSendOptions(t *testing.T) {
	options, err := parseMultipartSendOptions(`{"presence":"recording","delay":1250}`)
	if err != nil || options.Presence != "recording" || options.Delay != 1250 {
		t.Fatalf("unexpected options: %#v %v", options, err)
	}
	for _, options := range []*sendOptions{
		{Presence: "online", Delay: 1000},
		{Presence: "composing", Delay: -1},
		{Presence: "recording", Delay: 60001},
	} {
		if err := validateSendOptions(options); err == nil {
			t.Fatalf("invalid options accepted: %#v", options)
		}
	}
	if _, err := parseMultipartSendOptions(`{"presence":"composing","unknown":true}`); err == nil {
		t.Fatal("unknown multipart option accepted")
	}
}

func TestWaitSendDelayHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := waitSendDelay(ctx, 1000)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("canceled delay did not stop promptly")
	}
}
