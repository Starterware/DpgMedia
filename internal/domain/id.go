package domain

import (
	"strings"

	"github.com/google/uuid"
)

const (
	idSeparator = "_"

	messageIDPrefix = "msg"
	mediaIDPrefix   = "med"
)

func NewMessageID() string {
	return newID(messageIDPrefix)
}

func NewMediaID() string {
	return newID(mediaIDPrefix)
}

func newID(prefix string) string {
	prefix = strings.TrimSuffix(prefix, idSeparator)
	if prefix == "" {
		return uuid.NewString()
	}
	return prefix + idSeparator + uuid.NewString()
}
