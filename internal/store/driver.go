package store

import (
	"fmt"
	"strings"
)

type Driver string

const (
	DriverLocal Driver = "local"
)

func ParseDriver(s string) (Driver, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(DriverLocal):
		return DriverLocal, nil
	default:
		return "", fmt.Errorf("unknown store driver %q, want one of %s", s, DriverLocal)
	}
}
