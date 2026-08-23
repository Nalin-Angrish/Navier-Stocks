// Package utils provides shared helper functions used by agents across the
// system.  Currently it contains Indian-market-time guard checks that the
// Risk Manager uses to gate trade execution windows.
package utils

import "time"

// IsMarketOpen returns true if now falls between 9:30 AM and 3:00 PM IST
// (the regular cash-market trading hours for NSE / BSE).  Returns false
// if the Asia/Kolkata timezone cannot be loaded.
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

// IsOpeningBuffer returns true if now falls within the 9:15–9:30 AM IST
// buffer window.  The Risk Manager blocks new intents during this period
// to avoid the volatile market-open gap.
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

// IsClosingCutoff returns true if now is at or past 3:00 PM IST.  The Risk
// Manager rejects new entries after this time and triggers the auto-square-off
// routine at 3:15 PM.
func IsClosingCutoff(now time.Time) bool {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return false
	}
	now = now.In(loc)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, loc)
	return !now.Before(cutoff)
}
