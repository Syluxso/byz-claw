package core

import (
	"testing"
	"time"
)

func TestQuietHoursSameDay(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	if !InQuietHours(now, "09:00", "17:00") {
		t.Fatal("expected inside")
	}
	if InQuietHours(now, "11:00", "17:00") {
		t.Fatal("expected outside")
	}
}

func TestQuietHoursOvernight(t *testing.T) {
	late := time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)
	early := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if !InQuietHours(late, "23:00", "07:00") {
		t.Fatal("23:30 should be quiet")
	}
	if !InQuietHours(early, "23:00", "07:00") {
		t.Fatal("06:00 should be quiet")
	}
	if InQuietHours(mid, "23:00", "07:00") {
		t.Fatal("noon should not be quiet")
	}
}
