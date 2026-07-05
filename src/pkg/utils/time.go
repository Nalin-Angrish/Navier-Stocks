package utils

import "time"

func IsMarketOpen(now time.Time) bool {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return false
	}
	now = now.In(loc)
	open := time.Date(now.Year(), now.Month(), now.Day(), 9, 30, 0, 0, loc)
	close := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, loc)
	return now.After(open) && now.Before(close)
}

func IsOpeningBuffer(now time.Time) bool {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return false
	}
	now = now.In(loc)
	bufStart := time.Date(now.Year(), now.Month(), now.Day(), 9, 15, 0, 0, loc)
	bufEnd := time.Date(now.Year(), now.Month(), now.Day(), 9, 30, 0, 0, loc)
	return now.After(bufStart) && now.Before(bufEnd)
}

func IsClosingCutoff(now time.Time) bool {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return false
	}
	now = now.In(loc)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, loc)
	return now.After(cutoff)
}
