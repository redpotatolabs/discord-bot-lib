package botlib

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func runWatchdog(t *testing.T, probe func() bool, stall, within time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), within)
	defer cancel()
	wd := &watchdog{probe: probe, interval: 5 * time.Millisecond, stall: stall}
	return wd.run(ctx)
}

func TestWatchdog_HealthyNeverTrips(t *testing.T) {
	err := runWatchdog(t, func() bool { return true }, 50*time.Millisecond, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("healthy session tripped: %v", err)
	}
}

func TestWatchdog_UnhealthyTrips(t *testing.T) {
	err := runWatchdog(t, func() bool { return false }, 50*time.Millisecond, 2*time.Second)
	if !errors.Is(err, ErrGatewayStalled) {
		t.Fatalf("err = %v, want ErrGatewayStalled", err)
	}
	if !strings.Contains(err.Error(), "heartbeat") {
		t.Errorf("err = %q, want disconnected cause", err)
	}
}

func TestWatchdog_BlockedProbeTripsWithoutStacking(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	probe := func() bool {
		calls.Add(1)
		<-release
		return true
	}
	err := runWatchdog(t, probe, 50*time.Millisecond, 2*time.Second)
	if !errors.Is(err, ErrGatewayStalled) {
		t.Fatalf("err = %v, want ErrGatewayStalled", err)
	}
	if !strings.Contains(err.Error(), "session lock held") {
		t.Errorf("err = %q, want lock-held cause", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("probe started %d times while blocked, want 1", n)
	}
}

func TestWatchdog_RecoversBeforeStall(t *testing.T) {
	start := time.Now()
	// Unhealthy for well under the stall window, then healthy again.
	probe := func() bool { return time.Since(start) > 60*time.Millisecond }
	err := runWatchdog(t, probe, 200*time.Millisecond, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("recovered session tripped: %v", err)
	}
}

func TestSessionHealthy(t *testing.T) {
	cases := []struct {
		name  string
		ready bool
		ack   time.Duration // age of LastHeartbeatAck
		want  bool
	}{
		{"connected", true, 10 * time.Second, true},
		{"not ready", false, 10 * time.Second, false},
		{"stale ack", true, maxHeartbeatAckAge + time.Second, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &discordgo.Session{DataReady: tc.ready, LastHeartbeatAck: time.Now().Add(-tc.ack)}
			if got := sessionHealthy(s); got != tc.want {
				t.Errorf("sessionHealthy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSessionHealthy_BlocksWhileLockHeld(t *testing.T) {
	s := &discordgo.Session{DataReady: true, LastHeartbeatAck: time.Now()}
	s.Lock() // as discordgo's Open does when it deadlocks on Op 7
	done := make(chan bool, 1)
	go func() { done <- sessionHealthy(s) }()
	select {
	case <-done:
		t.Fatal("sessionHealthy returned while the session lock was held")
	case <-time.After(50 * time.Millisecond):
	}
	s.Unlock()
	if !<-done {
		t.Error("sessionHealthy = false after lock released, want true")
	}
}
