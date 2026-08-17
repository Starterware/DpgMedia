// Package id generates prefixed identifiers such as
// "msg_0b7c1f2e-4f83-4a1a-9c2b-3f7f6e5d4c3b".
//
// The prefix makes an identifier self-describing in logs and error reports —
// you can tell what a bare id refers to without knowing which field it came
// from — while the UUID supplies the uniqueness.
package id

import (
	"strings"

	"github.com/google/uuid"
)

// Separator sits between the prefix and the UUID.
const Separator = "_"

// New returns prefix joined to a new random (v4) UUID, for example
// New("msg") returns "msg_0b7c1f2e-4f83-4a1a-9c2b-3f7f6e5d4c3b".
//
// A prefix that already ends in the separator does not produce a doubled one,
// and an empty prefix yields a bare UUID rather than a leading separator.
func New(prefix string) string {
	prefix = strings.TrimSuffix(prefix, Separator)
	if prefix == "" {
		return uuid.NewString()
	}
	return prefix + Separator + uuid.NewString()
}
