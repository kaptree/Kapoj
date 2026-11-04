package contest

import "time"

type Status string

const (
	StatusUpcoming Status = "upcoming"
	StatusRunning  Status = "running"
	StatusEnded    Status = "ended"
)

func StatusAt(start, end, now time.Time) Status {
	switch {
	case now.Before(start):
		return StatusUpcoming
	case now.Before(end):
		return StatusRunning
	default:
		return StatusEnded
	}
}
