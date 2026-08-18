package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDriver(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Driver
		wantErr bool
	}{
		{name: "local", input: "local", want: DriverLocal},
		{name: "uppercase", input: "LOCAL", want: DriverLocal},
		{name: "padded", input: "  local  ", want: DriverLocal},
		{name: "unknown", input: "dynamodb", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDriver(tc.input)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown store driver")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
