// Package wslclient contains the platform-neutral, security-sensitive pieces
// of the Windows launcher's WSL boundary. The Windows adapter owns process and
// UI APIs; this package owns untrusted text/protocol validation.
package wslclient

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf16"
)

const MaxOutputBytes = 16 << 10

type Target struct {
	Distribution string `json:"distribution"`
	User         string `json:"user"`
	PayloadPath  string `json:"payload_path"`
}

func (t Target) Validate() error {
	if err := validateArgument("distribution", t.Distribution, 128); err != nil {
		return err
	}
	if err := validateArgument("user", t.User, 256); err != nil {
		return err
	}
	if err := ValidateLinuxPath("payload path", t.PayloadPath); err != nil {
		return err
	}
	clean := path.Clean(t.PayloadPath)
	if clean == "/" {
		return errors.New("payload path must not be the Linux filesystem root")
	}
	if clean == "/mnt" || strings.HasPrefix(clean, "/mnt/") {
		return errors.New("payload path must be in the WSL filesystem, not under /mnt")
	}
	return nil
}

func ValidateLinuxPath(label, value string) error {
	if value == "" || !path.IsAbs(value) {
		return fmt.Errorf("%s must be an absolute Linux path", label)
	}
	if strings.IndexByte(value, 0) >= 0 || containsControl(value) {
		return fmt.Errorf("%s contains control characters", label)
	}
	if len(value) > 4096 {
		return fmt.Errorf("%s is too long", label)
	}
	return nil
}

func (t Target) HelperPath() string {
	return path.Join(path.Clean(t.PayloadPath), "cats-wsl-host")
}

func HealthArgs(t Target) []string {
	return []string{
		"--distribution", t.Distribution,
		"--user", t.User,
		"--exec", t.HelperPath(),
		"--health-json",
	}
}

func LaunchArgs(t Target, port int, launchID, startDir string) ([]string, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid loopback port %d", port)
	}
	if !validLaunchID(launchID) {
		return nil, errors.New("launch id must be 1-64 ASCII letters, digits, underscores, or hyphens")
	}
	args := []string{
		"--distribution", t.Distribution,
		"--user", t.User,
		"--exec", t.HelperPath(),
		"--port", fmt.Sprint(port),
		"--launch-id", launchID,
	}
	if startDir != "" {
		if err := ValidateLinuxPath("start directory", startDir); err != nil {
			return nil, err
		}
		args = append(args, "--start-dir", path.Clean(startDir))
	}
	return args, nil
}

func DecodeDistributionList(data []byte) ([]string, error) {
	if len(data) > MaxOutputBytes {
		return nil, errors.New("WSL distribution list is too large")
	}
	text, err := decodeWSLText(data)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var result []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		name := strings.TrimSpace(strings.Trim(line, "\x00\ufeff"))
		if name == "" || seen[name] {
			continue
		}
		if err := validateArgument("distribution", name, 128); err != nil {
			return nil, err
		}
		seen[name] = true
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil, errors.New("WSL reported no installed distributions")
	}
	return result, nil
}

func decodeWSLText(data []byte) (string, error) {
	utf16Text := len(data) >= 2 && (data[0] == 0xff && data[1] == 0xfe)
	if !utf16Text {
		for index := 1; index < len(data); index += 2 {
			if data[index] == 0 {
				utf16Text = true
				break
			}
		}
	}
	if !utf16Text {
		return string(data), nil
	}
	if len(data)%2 != 0 {
		return "", errors.New("WSL returned truncated UTF-16 output")
	}
	units := make([]uint16, 0, len(data)/2)
	for index := 0; index < len(data); index += 2 {
		units = append(units, uint16(data[index])|uint16(data[index+1])<<8)
	}
	if len(units) > 0 && units[0] == 0xfeff {
		units = units[1:]
	}
	return string(utf16.Decode(units)), nil
}

func validateArgument(label, value string, max int) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", label)
	}
	if trimmed != value {
		return fmt.Errorf("%s has leading or trailing whitespace", label)
	}
	if len(value) > max {
		return fmt.Errorf("%s is too long", label)
	}
	if strings.IndexByte(value, 0) >= 0 || containsControl(value) {
		return fmt.Errorf("%s contains control characters", label)
	}
	return nil
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func validLaunchID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
