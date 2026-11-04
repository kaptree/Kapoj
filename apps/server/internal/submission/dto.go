package submission

// Created is returned by POST /submissions.
type Created struct {
	ID     int64  `json:"id"`
	Status Status `json:"status"`
}

func ToCreated(sub *Submission) Created {
	return Created{ID: sub.ID, Status: sub.Status}
}
