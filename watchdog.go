package botlib

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ErrGatewayStalled is returned by Run when the watchdog finds the gateway
// connection unhealthy for longer than RunConfig.WatchdogStall. Callers
// should exit non-zero so their supervisor restarts the process.
var ErrGatewayStalled = errors.New("botlib: gateway connection stalled")

const (
	defaultWatchdogStall = 5 * time.Minute
	watchdogInterval     = 30 * time.Second
	maxHeartbeatAckAge   = 3 * time.Minute // Discord heartbeats every ~41s
)

// sessionHealthy reports whether s holds a live, acknowledged gateway
// connection. It blocks while the session mutex is held.
//
// discordgo v0.29.0 can deadlock that mutex permanently: if Discord answers
// a reconnect's Open with Op 7, onEvent calls CloseWithCode while Open still
// holds the lock. Heartbeats, reconnects and event dispatch all stop, nothing
// is logged, and the process stays up. The watchdog exists to catch that, as
// well as a reconnect loop that never recovers.
func sessionHealthy(s *discordgo.Session) bool {
	s.RLock()
	defer s.RUnlock()
	return s.DataReady && time.Since(s.LastHeartbeatAck) < maxHeartbeatAckAge
}

type watchdog struct {
	probe    func() bool // may block indefinitely
	interval time.Duration
	stall    time.Duration
}

// run probes every interval and returns ErrGatewayStalled once no probe has
// reported healthy for stall. A probe that blocks is left pending (never
// stacked) and counts as unhealthy. Returns nil when ctx is cancelled.
func (w *watchdog) run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	lastHealthy := time.Now()
	var pending chan bool // nil (never ready) while no probe is in flight
	for {
		var cause string
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if pending == nil {
				pending = make(chan bool, 1)
				go func(ch chan<- bool) { ch <- w.probe() }(pending)
				continue
			}
			cause = "session lock held"
		case ok := <-pending:
			pending = nil
			if ok {
				lastHealthy = time.Now()
				continue
			}
			cause = "disconnected or missing heartbeat ACKs"
		}

		if stalled := time.Since(lastHealthy); stalled >= w.stall {
			return fmt.Errorf("%w: unhealthy for %s (%s)", ErrGatewayStalled, stalled.Round(time.Second), cause)
		}
	}
}
