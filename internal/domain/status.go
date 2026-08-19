package domain

type Status string

const (
	StatusReady                Status = "READY"
	StatusPendingTranscription Status = "PENDING_TRANSCRIPTION"
	StatusFailedTranscription  Status = "FAILED_TRANSCRIPTION"
)

func (s Status) Valid() bool {
	switch s {
	case StatusReady, StatusPendingTranscription, StatusFailedTranscription:
		return true
	default:
		return false
	}
}

func (s Status) Failed() bool {
	return s == StatusFailedTranscription
}

func InitialStatus(t Type) Status {
	if t == TypeAudio {
		return StatusPendingTranscription
	}
	return StatusReady
}
