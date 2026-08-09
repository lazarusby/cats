package wslclient

import "regexp"

var sensitiveLogValue = regexp.MustCompile(`(?i)\b(password|token|secret|authorization)\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`)

// RedactLogChunk removes common credential assignments from helper/launcher
// diagnostics. Clipboard data and environment dumps are never intentionally
// written; this is a final boundary for accidental token-shaped log messages.
func RedactLogChunk(data []byte) []byte {
	return sensitiveLogValue.ReplaceAll(data, []byte(`${1}=[REDACTED]`))
}
