package skill

import "strings"

// SanitizeFileName removes characters that are illegal in file names,
// trims whitespace, and returns a default value if the result is empty.
func SanitizeFileName(name string) string {
	safe := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return -1
		default:
			return r
		}
	}, strings.TrimSpace(name))
	if safe == "" {
		return "unnamed"
	}
	return safe
}
