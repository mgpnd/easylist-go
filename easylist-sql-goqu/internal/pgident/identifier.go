// Package pgident implements shared PostgreSQL identifier normalization.
package pgident

import "unicode/utf8"

// Truncate matches the PostgreSQL parser's 63-byte identifier limit without splitting a
// UTF-8 character. Call after decoding quotes and folding unquoted ASCII case.
func Truncate(name string) string {
	const maxIdentifierBytes = 63
	if len(name) <= maxIdentifierBytes {
		return name
	}
	end := maxIdentifierBytes
	for end > 0 && !utf8.RuneStart(name[end]) {
		end--
	}
	return name[:end]
}
