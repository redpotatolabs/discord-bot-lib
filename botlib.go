package botlib

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/epistemi/beacon/pkg/beacon"
)

// App is the shared bot lifecycle harness. It owns the discordgo session,
// the beacon reporter, and the dispatcher wiring. Caller code owns its
// own Bot struct, commands, and store.
type App struct {
	Session  *discordgo.Session
	Reporter *BeaconReporter
	logger   Logger
}

// New constructs an App with a discordgo session built from a bot token
// and the requested intents. It does not call session.Open — see Run.
func New(token string, intents discordgo.Intent) (*App, error) {
	if token == "" {
		return nil, errors.New("botlib: token is required")
	}
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("botlib: discordgo.New: %w", err)
	}
	s.Identify.Intents = intents
	logger := StdLogger()
	return &App{
		Session:  s,
		Reporter: &BeaconReporter{Logger: logger},
		logger:   logger,
	}, nil
}

// SetLogger replaces the App's logger. Pass StdLogger() (the default), a
// zaplog-backed logger, or any Logger implementation. Also updates the
// Reporter's logger so the two stay in sync.
func (a *App) SetLogger(l Logger) {
	if l == nil {
		return
	}
	a.logger = l
	if a.Reporter != nil {
		a.Reporter.Logger = l
	}
}

// SetBeacon attaches a beacon client. Subsequent Reporter.Report calls
// post to beacon. Pass nil to disable.
func (a *App) SetBeacon(bc *beacon.Client) {
	if a.Reporter == nil {
		a.Reporter = &BeaconReporter{Logger: a.logger}
	}
	a.Reporter.Client = bc
}

func (a *App) logger_() Logger { return a.logger }

// (logger is exposed as a method for tests and internal helpers.)
func (a *App) Logger() Logger { return a.logger }

// Worker is a long-lived background task scheduled by App.Run. The given
// context is cancelled when the App enters shutdown; workers should return
// shortly after.
type Worker func(ctx context.Context) error

// Shutdowner is any value with a Shutdown(ctx) method. http.Server
// already satisfies it.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// RunConfig controls App.Run.
type RunConfig struct {
	// Workers are launched as goroutines after Session.Open() succeeds
	// and cancelled when shutdown begins.
	Workers []Worker
	// Shutdowners are invoked sequentially during graceful shutdown
	// (each with a 5-second timeout) before Session.Close.
	Shutdowners []Shutdowner
	// OnReady runs once after Session.Open succeeds, before the signal
	// wait. Useful for "bot is running as X" logging.
	OnReady func(*discordgo.Session)
	// ShutdownTimeout caps the total drain time. Defaults to 10s.
	ShutdownTimeout time.Duration
	// WatchdogStall is how long the gateway connection may stay unhealthy
	// (disconnected, missing heartbeat ACKs, or session lock held) before
	// Run gives up and returns ErrGatewayStalled. Defaults to 5m; negative
	// disables the watchdog.
	WatchdogStall time.Duration
}

// Run opens the discordgo session, launches workers, runs OnReady, and
// blocks on SIGINT/SIGTERM (or ctx.Done()). On shutdown it cancels
// workers, drains each Shutdowner, and closes the session.
//
// Worker errors are reported through Reporter but never stop the bot —
// matches the existing "web UI failure must not kill the bot" stance.
//
// A gateway watchdog runs alongside the workers. If the connection stays
// unhealthy for WatchdogStall, Run shuts down without closing the session
// (which may be deadlocked) and returns ErrGatewayStalled.
func (a *App) Run(ctx context.Context, cfg RunConfig) error {
	if err := a.Session.Open(); err != nil {
		return fmt.Errorf("botlib: session.Open: %w", err)
	}
	var stallErr error
	defer func() {
		// Close takes the session mutex, which a stalled session may never
		// release.
		if stallErr == nil {
			a.Session.Close()
		}
	}()

	if cfg.OnReady != nil {
		cfg.OnReady(a.Session)
	}

	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	stalled := make(chan error, 1)
	if cfg.WatchdogStall >= 0 {
		stall := cfg.WatchdogStall
		if stall == 0 {
			stall = defaultWatchdogStall
		}
		wd := &watchdog{
			probe:    func() bool { return sessionHealthy(a.Session) },
			interval: min(watchdogInterval, stall/4),
			stall:    stall,
		}
		go func() {
			if err := wd.run(workerCtx); err != nil {
				stalled <- err
			}
		}()
	}

	var wg sync.WaitGroup
	for _, w := range cfg.Workers {
		wg.Add(1)
		go func(w Worker) {
			defer wg.Done()
			if err := w(workerCtx); err != nil {
				a.Reporter.Report(workerCtx, "error",
					fmt.Sprintf("worker exited with error: %v", err))
			}
		}(w)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
	case <-ctx.Done():
	case stallErr = <-stalled:
	}

	cancelWorkers()

	timeout := cfg.ShutdownTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if stallErr != nil {
		a.Reporter.Report(shutdownCtx, "error",
			fmt.Sprintf("%v; exiting so the supervisor restarts the bot", stallErr))
	}

	for _, sd := range cfg.Shutdowners {
		stepCtx, stepCancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		if err := sd.Shutdown(stepCtx); err != nil {
			a.logger.Warnf("shutdown error: %v", err)
		}
		stepCancel()
	}

	if stallErr == nil {
		wg.Wait()
		return nil
	}
	// A worker blocked on the stalled session must not keep the process up.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		a.logger.Warnf("workers still running at shutdown timeout; exiting anyway")
	}
	return stallErr
}
