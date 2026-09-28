package systemclock

import (
	"testing"
	"time"
)

func TestClockNowIsNonZeroAndAdvances(t *testing.T) {
	clock := New()
	before := time.Now().Add(-time.Second)
	got := clock.Now()
	after := time.Now().Add(time.Second)
	if got.Before(before) || got.After(after) {
		t.Fatalf("Now()=%v outside observation window [%v,%v]", got, before, after)
	}
}
