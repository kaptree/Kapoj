package submission

import (
	"context"
	"time"
)

const (
	activityDateLayout = "2006-01-02"
	calendarDays       = 365
)

// ActivityDay is one day of the contribution calendar.
type ActivityDay struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// UserActivity is a public profile's submission statistics and 365-day code
// contribution calendar. SubmissionCount includes objective answers;
// ContributionTotal and Days only count code submissions.
type UserActivity struct {
	SubmissionCount      int64         `json:"submissionCount"`
	AcceptedProblemCount int64         `json:"acceptedProblemCount"`
	ContributionTotal    int64         `json:"contributionTotal"`
	StartDate            string        `json:"startDate"`
	EndDate              string        `json:"endDate"`
	CurrentStreak        int           `json:"currentStreak"`
	LongestStreak        int           `json:"longestStreak"`
	Days                 []ActivityDay `json:"days"`
}

// UserActivity aggregates a user's submissions and builds their contribution
// calendar. Day boundaries use the server's local timezone; the calendar ends
// at local midnight today and covers the previous 364 days.
func (s *Service) UserActivity(ctx context.Context, userID int64) (*UserActivity, error) {
	now := time.Now().In(time.Local)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	start := end.AddDate(0, 0, -(calendarDays - 1))

	total, accepted, err := s.repo.CountUserActivity(ctx, userID)
	if err != nil {
		return nil, err
	}
	times, err := s.repo.ListCodeSubmissionTimes(ctx, userID, start)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64, calendarDays)
	for _, created := range times {
		counts[created.In(time.Local).Format(activityDateLayout)]++
	}

	days := make([]ActivityDay, 0, calendarDays)
	var contributionTotal int64
	var current, longest, run int
	for i := 0; i < calendarDays; i++ {
		date := start.AddDate(0, 0, i).Format(activityDateLayout)
		count := counts[date]
		days = append(days, ActivityDay{Date: date, Count: count})
		contributionTotal += count
		if count > 0 {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	// Current streak starts today; a day without code submissions breaks it.
	for i := len(days) - 1; i >= 0 && days[i].Count > 0; i-- {
		current++
	}

	return &UserActivity{
		SubmissionCount:      total,
		AcceptedProblemCount: accepted,
		ContributionTotal:    contributionTotal,
		StartDate:            start.Format(activityDateLayout),
		EndDate:              end.Format(activityDateLayout),
		CurrentStreak:        current,
		LongestStreak:        longest,
		Days:                 days,
	}, nil
}
