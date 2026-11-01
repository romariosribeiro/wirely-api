package engine

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
)

func TestCallDiagnosticsDoesNotLogSensitivePayloads(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	logger := callDiagnosticsLogger{instanceID: "instance"}
	secret := "SENSITIVE_CALL_KEY_AND_CONTENT"
	call := &waBinary.Node{Tag: "call", Attrs: waBinary.Attrs{"from": secret}, Content: []waBinary.Node{{Tag: "offer", Content: []byte(secret)}}}
	logger.Sub("Recv").Debugf("%s", call)
	logger.Sub("Send").Debugf("%s", &waBinary.Node{Tag: "call", Content: []waBinary.Node{{Tag: "reject"}}})
	logger.Sub("Recv").Debugf("%s", &waBinary.Node{Tag: "message", Content: []byte(secret)})
	logger.Debugf("Errored frame hex: %s", secret)
	logger.Warnf("Failed to decode node in frame: %v", secret)
	logger.Warnf("unrelated warning: %v", secret)
	logger.Errorf("Failed to parse call: %v", secret)
	logger.Infof("ignore: %s", secret)
	text := output.String()
	if strings.Contains(text, secret) || strings.Contains(text, "unrelated warning") || strings.Contains(text, "Errored frame hex") {
		t.Fatalf("sensitive or unrelated data logged: %s", text)
	}
	if !strings.Contains(text, `"direction":"Recv"`) || !strings.Contains(text, `"direction":"Send"`) ||
		!strings.Contains(text, `"tags":["offer"]`) || !strings.Contains(text, `"tags":["reject"]`) ||
		!strings.Contains(text, "Failed to decode node in frame") || !strings.Contains(text, "Failed to parse call") {
		t.Fatalf("diagnostic information missing: %s", text)
	}
}
