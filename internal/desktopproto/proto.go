// Package desktopproto defines the small lifecycle protocol spoken between the
// native Windows launcher and cats-wsl-host. It is deliberately independent of
// the browser, orchestration, control, and hook protocols: terminal traffic and
// commands never belong on this channel.
package desktopproto

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

// ProtocolVersion changes only when the launcher/host lifecycle contract makes
// a breaking change.
const ProtocolVersion = 1

// MaxRecordBytes is the largest JSON object, excluding its terminating newline,
// accepted on the lifecycle channel. Legitimate records are normally a few
// hundred bytes; the larger ceiling leaves room for useful diagnostics without
// permitting an untrusted helper to grow launcher memory without bound.
const MaxRecordBytes = 16 << 10

// Type is the JSON "type" discriminator.
type Type string

const (
	// Helper to launcher.
	TypeStarting Type = "starting"
	TypeReady    Type = "ready"
	TypeError    Type = "error"
	TypeStopped  Type = "stopped"

	// Launcher to helper.
	TypeStop Type = "stop"
)

var (
	ErrMalformed      = errors.New("desktopproto: malformed record")
	ErrTruncated      = errors.New("desktopproto: truncated record")
	ErrRecordTooLarge = errors.New("desktopproto: record too large")
	ErrUnknownVersion = errors.New("desktopproto: unknown version")
	ErrUnknownType    = errors.New("desktopproto: unknown record type")
)

// HelperRecord is one host-to-launcher lifecycle update. Fields not used by its
// Type must remain empty; this keeps the control channel narrow and makes it
// difficult to accidentally add terminal data or commands to it.
type HelperRecord struct {
	V          int    `json:"v"`
	Type       Type   `json:"type"`
	AppVersion string `json:"app_version,omitempty"`
	Addr       string `json:"addr,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Stage      string `json:"stage,omitempty"`
	Message    string `json:"message,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// LauncherRecord is one launcher-to-host request. Version 1 intentionally
// supports only stop; it is not a general remote-command envelope.
type LauncherRecord struct {
	V    int  `json:"v"`
	Type Type `json:"type"`
}

func NewStarting(appVersion string, pid int) HelperRecord {
	return HelperRecord{V: ProtocolVersion, Type: TypeStarting, AppVersion: appVersion, PID: pid}
}

func NewReady(appVersion, addr string, pid int) HelperRecord {
	return HelperRecord{V: ProtocolVersion, Type: TypeReady, AppVersion: appVersion, Addr: addr, PID: pid}
}

func NewError(stage, message string) HelperRecord {
	return HelperRecord{V: ProtocolVersion, Type: TypeError, Stage: stage, Message: message}
}

func NewStopped(reason string) HelperRecord {
	return HelperRecord{V: ProtocolVersion, Type: TypeStopped, Reason: reason}
}

func NewStop() LauncherRecord {
	return LauncherRecord{V: ProtocolVersion, Type: TypeStop}
}

// Validate checks a helper record's version, type, required fields, and the
// absence of fields belonging to another record type.
func (r HelperRecord) Validate() error {
	if err := validateEnvelope(r.V, r.Type); err != nil {
		return err
	}
	switch r.Type {
	case TypeStarting:
		if r.AppVersion == "" || r.PID <= 0 {
			return fmt.Errorf("%w: starting requires app_version and positive pid", ErrMalformed)
		}
		if r.Addr != "" || r.Stage != "" || r.Message != "" || r.Reason != "" {
			return fmt.Errorf("%w: starting contains unrelated fields", ErrMalformed)
		}
	case TypeReady:
		if r.AppVersion == "" || r.PID <= 0 || r.Addr == "" {
			return fmt.Errorf("%w: ready requires app_version, addr, and positive pid", ErrMalformed)
		}
		if err := validateLoopbackAddr(r.Addr); err != nil {
			return err
		}
		if r.Stage != "" || r.Message != "" || r.Reason != "" {
			return fmt.Errorf("%w: ready contains unrelated fields", ErrMalformed)
		}
	case TypeError:
		if r.Stage == "" || r.Message == "" {
			return fmt.Errorf("%w: error requires stage and message", ErrMalformed)
		}
		if r.AppVersion != "" || r.Addr != "" || r.PID != 0 || r.Reason != "" {
			return fmt.Errorf("%w: error contains unrelated fields", ErrMalformed)
		}
	case TypeStopped:
		if r.Reason == "" {
			return fmt.Errorf("%w: stopped requires reason", ErrMalformed)
		}
		if r.AppVersion != "" || r.Addr != "" || r.PID != 0 || r.Stage != "" || r.Message != "" {
			return fmt.Errorf("%w: stopped contains unrelated fields", ErrMalformed)
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnknownType, r.Type)
	}
	return nil
}

// Validate checks a launcher record. No payload fields exist because stop is
// the only launcher request in protocol version 1.
func (r LauncherRecord) Validate() error {
	if err := validateEnvelope(r.V, r.Type); err != nil {
		return err
	}
	if r.Type != TypeStop {
		return fmt.Errorf("%w: %q", ErrUnknownType, r.Type)
	}
	return nil
}

func validateEnvelope(version int, typ Type) error {
	if version != ProtocolVersion {
		return fmt.Errorf("%w: %d", ErrUnknownVersion, version)
	}
	if typ == "" {
		return fmt.Errorf("%w: empty", ErrUnknownType)
	}
	return nil
}

func validateLoopbackAddr(addr string) error {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" {
		return fmt.Errorf("%w: ready addr must be 127.0.0.1:<port>", ErrMalformed)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%w: ready addr has invalid port", ErrMalformed)
	}
	return nil
}
