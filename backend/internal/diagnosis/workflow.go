package diagnosis

import "errors"

var ErrInvalidTransition = errors.New("invalid diagnosis status transition")
var ErrInvalidFeedback = errors.New("invalid diagnosis feedback verdict")
var ErrAlreadyAssigned = errors.New("diagnosis is already assigned to this user")

func CanTransition(from, to string) bool {
	switch from {
	case "open":
		return to == "confirmed" || to == "dismissed"
	case "confirmed":
		return to == "resolved" || to == "dismissed"
	case "resolved", "dismissed":
		return to == "open"
	default:
		return false
	}
}

func ValidFeedbackVerdict(verdict string) bool {
	return verdict == "accurate" || verdict == "inaccurate" || verdict == "uncertain"
}
