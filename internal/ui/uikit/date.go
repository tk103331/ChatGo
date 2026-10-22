package uikit

import (
	"fmt"
	"time"
)

// Date is a calendar day without clock or location; zero means none.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the day t falls on in t's own location, without converting it.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// IsZero reports whether d is the empty date.
func (d Date) IsZero() bool { return d == Date{} }

// Civil returns noon on d in the local location, a safe stand-in for the day.
func (d Date) Civil() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.Local)
}

// Valid reports whether d names a real day.
func (d Date) Valid() bool { return !d.IsZero() && DateOf(d.Civil()) == d }

// AddDays returns the day n days after d.
func (d Date) AddDays(n int) Date { return DateOf(d.Civil().AddDate(0, 0, n)) }

// Weekday returns the day of the week d falls on.
func (d Date) Weekday() time.Weekday { return d.Civil().Weekday() }

// String formats d as 2006-01-02, or "" when empty.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}
