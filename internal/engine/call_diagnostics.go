package engine

import (
	"log/slog"
	"strings"

	waBinary "go.mau.fi/whatsmeow/binary"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// Inspect typed call nodes before event parsing. Never stringify nodes: they may
// contain call keys, phone numbers, or unrelated message contents.
type callDiagnosticsLogger struct {
	instanceID string
	module     string
}

var _ waLog.Logger = callDiagnosticsLogger{}

func (l callDiagnosticsLogger) Sub(module string) waLog.Logger {
	if l.module != "" {
		module = l.module + "/" + module
	}
	return callDiagnosticsLogger{instanceID: l.instanceID, module: module}
}

func (l callDiagnosticsLogger) Debugf(format string, args ...any) {
	if format != "%s" || len(args) != 1 || (l.module != "Recv" && l.module != "Send") {
		return
	}
	node, ok := args[0].(*waBinary.Node)
	if !ok || node == nil || node.Tag != "call" {
		return
	}
	tags := make([]string, 0, len(node.GetChildren()))
	for _, child := range node.GetChildren() {
		tags = append(tags, child.Tag)
	}
	slog.Info("WhatsApp call signaling", "instance_id", l.instanceID, "direction", l.module, "tags", tags)
}

func (l callDiagnosticsLogger) logWarning(level slog.Level, format string) {
	// Keep only operation templates, not formatted arguments or raw frames.
	if !strings.Contains(strings.ToLower(format), "call") &&
		format != "Failed to decompress frame: %v" &&
		format != "Failed to decode node in frame: %v" &&
		format != "Failed to send post-connect passive IQ: %v" {
		return
	}
	slog.Log(nil, level, "WhatsApp signaling diagnostic", "instance_id", l.instanceID, "module", l.module, "operation", format)
}

func (l callDiagnosticsLogger) Warnf(format string, _ ...any) {
	l.logWarning(slog.LevelWarn, format)
}
func (l callDiagnosticsLogger) Errorf(format string, _ ...any) {
	l.logWarning(slog.LevelError, format)
}
func (l callDiagnosticsLogger) Infof(string, ...any) {}
