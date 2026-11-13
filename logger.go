// Package botlib is the shared Discord-bot lifecycle harness used by
// redpotatolabs services (beacon, vault, tater). It owns the bits that don't
// belong in any one bot: discordgo session boot, per-guild command
// registration, the InteractionCreate dispatcher, beacon-backed error
// reporting, the standard config validation pattern, and an opinionated
// Run loop with graceful shutdown.
//
// Domain code (commands, stores, business logic) stays per-bot.
// botlib depends on small interfaces (CommandRegistrar, GuildLister,
// Logger, Shutdowner), not on the caller's Bot struct.
package botlib

import (
	"log"
	"os"
)

// Logger is the small interface botlib uses for diagnostics. Bots can
// implement it themselves or use the stdlib-log-backed default returned
// by StdLogger().
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// StdLogger returns a Logger backed by the stdlib `log` package, writing
// to stderr. Used as the default when callers don't supply their own.
func StdLogger() Logger {
	return &stdLogger{l: log.New(os.Stderr, "", log.LstdFlags)}
}

type stdLogger struct{ l *log.Logger }

func (s *stdLogger) Debugf(f string, a ...any) { s.l.Printf("DEBUG "+f, a...) }
func (s *stdLogger) Infof(f string, a ...any)  { s.l.Printf("INFO  "+f, a...) }
func (s *stdLogger) Warnf(f string, a ...any)  { s.l.Printf("WARN  "+f, a...) }
func (s *stdLogger) Errorf(f string, a ...any) { s.l.Printf("ERROR "+f, a...) }
