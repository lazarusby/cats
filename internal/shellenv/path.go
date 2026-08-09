// Package shellenv contains the platform-neutral pieces of deriving a user's
// shell environment. Platform launchers remain responsible for choosing and
// invoking the shell; captured output is treated as data, never evaluated.
package shellenv

import "strings"

// BetweenMarkers extracts the text fenced by the first two occurrences of
// marker. Login shells may print arbitrary rc-file noise around those markers.
func BetweenMarkers(s, marker string) string {
	start := strings.Index(s, marker)
	if start < 0 {
		return ""
	}
	rest := s[start+len(marker):]
	end := strings.Index(rest, marker)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// MergePATH puts the derived shell entries first and appends inherited entries
// not already present. Empty entries are discarded.
func MergePATH(derived, inherited string) string {
	seen := make(map[string]bool)
	var merged []string
	for _, list := range []string{derived, inherited} {
		for _, entry := range strings.Split(list, ":") {
			if entry == "" || seen[entry] {
				continue
			}
			seen[entry] = true
			merged = append(merged, entry)
		}
	}
	return strings.Join(merged, ":")
}
