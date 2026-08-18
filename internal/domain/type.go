package domain

import (
	"fmt"
	"strings"
)

type Type string

const (
	TypeText    Type = "TEXT"
	TypeAudio   Type = "AUDIO"
	TypeVideo   Type = "VIDEO"
	TypePicture Type = "PICTURE"
)

const TypeNames = "TEXT, AUDIO, VIDEO, PICTURE"

func (t Type) Valid() bool {
	switch t {
	case TypeText, TypeAudio, TypeVideo, TypePicture:
		return true
	default:
		return false
	}
}

func (t Type) RequiresMedia() bool {
	return t != TypeText
}

func ParseType(s string) (Type, error) {
	parsed := Type(strings.ToUpper(strings.TrimSpace(s)))
	if !parsed.Valid() {
		return "", fmt.Errorf("unknown message type %q, want one of %s", s, TypeNames)
	}
	return parsed, nil
}
