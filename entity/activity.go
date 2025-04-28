package entity

import "time"

type ActivityRecord struct {
	ID          int64
	ProcessName string
	WindowTitle string
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
}
