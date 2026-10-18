//go:build !darwin

package main

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNextDelay(t *testing.T) {
	cases := []struct {
		prev   time.Duration
		code   int
		uptime time.Duration
		want   time.Duration
	}{
		{0, 1, time.Second, 10 * time.Second},
		{10 * time.Second, 1, time.Second, 20 * time.Second},
		{160 * time.Second, 1, time.Second, 5 * time.Minute},
		{5 * time.Minute, 1, time.Second, 5 * time.Minute},
		{5 * time.Minute, 1, 11 * time.Minute, 10 * time.Second}, // a long run resets the backoff
		{40 * time.Second, exitRestart, time.Second, 0},
	}
	for _, c := range cases {
		if got := nextDelay(c.prev, c.code, c.uptime); got != c.want {
			t.Errorf("nextDelay(%s, %d, %s) = %s want %s", c.prev, c.code, c.uptime, got, c.want)
		}
	}
}

// fakeRun drives superviseLoop with scripted exit codes.
func fakeRun(codes []int, startErrs int, stopAfter int) (starts int, sleeps []time.Duration, code int) {
	clock := time.Unix(0, 0)
	runs := 0
	l := superviseLoop{
		start: func() (func() int, error) {
			starts++
			if startErrs > 0 {
				startErrs--
				return nil, errors.New("no exe")
			}
			c := codes[runs]
			runs++
			return func() int { clock = clock.Add(time.Second); return c }, nil
		},
		stopped: func() bool { return stopAfter >= 0 && starts >= stopAfter },
		sleep:   func(d time.Duration) bool { sleeps = append(sleeps, d); clock = clock.Add(d); return true },
		now:     func() time.Time { return clock },
		logf:    func(string, ...any) {},
	}
	return starts, sleeps, l.run()
}

func TestSuperviseRestartsUntilCleanExit(t *testing.T) {
	starts, sleeps, code := fakeRun([]int{1, 1, exitRestart, 0}, 0, -1)
	if code != exitOK || starts != 4 || fmt.Sprint(sleeps) != "[10s 20s]" {
		t.Fatalf("starts %d sleeps %v code %d", starts, sleeps, code)
	}
	starts, sleeps, _ = fakeRun([]int{0}, 2, -1)
	if starts != 3 || fmt.Sprint(sleeps) != "[10s 20s]" {
		t.Fatalf("start errors: starts %d sleeps %v", starts, sleeps)
	}
	// A stop request after the first start ends supervision without a restart.
	starts, _, code = fakeRun([]int{1, 1}, 0, 1)
	if starts != 1 || code != exitOK {
		t.Fatalf("stop: starts %d", starts)
	}
}
