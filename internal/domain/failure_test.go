package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFailureCodeValid(t *testing.T) {
	tests := []struct {
		code FailureCode
		want bool
	}{
		{FailureMediaUnavailable, true},
		{FailureTranscriptionError, true},
		{FailureCode("media_unavailable"), false},
		{FailureCode("TIMEOUT"), false},
		{FailureCode(""), false},
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.code.Valid())
		})
	}
}

func TestFailureValidate(t *testing.T) {
	valid := Failure{
		Code:     FailureMediaUnavailable,
		Reason:   "media unavailable: media not found: med_1",
		FailedAt: time.Date(2026, 8, 18, 14, 2, 11, 0, time.UTC),
	}

	tests := []struct {
		name    string
		mutate  func(*Failure)
		wantMsg string
	}{
		{name: "complete failure"},
		{
			name:   "without a timestamp",
			mutate: func(f *Failure) { f.FailedAt = time.Time{} },
		},
		{
			name:    "unknown code",
			mutate:  func(f *Failure) { f.Code = "TIMEOUT" },
			wantMsg: `unknown code "TIMEOUT"`,
		},
		{
			name:    "missing code",
			mutate:  func(f *Failure) { f.Code = "" },
			wantMsg: `unknown code ""`,
		},
		{
			name:    "missing reason",
			mutate:  func(f *Failure) { f.Reason = "" },
			wantMsg: "reason is required",
		},
		{
			name:    "blank reason",
			mutate:  func(f *Failure) { f.Reason = "  \t" },
			wantMsg: "reason is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			failure := valid
			if tc.mutate != nil {
				tc.mutate(&failure)
			}

			err := failure.Validate()

			if tc.wantMsg == "" {
				assert.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, ErrInvalidFailure)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
