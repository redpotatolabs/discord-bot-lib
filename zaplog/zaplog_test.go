package zaplog

import (
	"testing"

	botlib "github.com/epistemi/discord-bot-lib"
)

// TestImplementsLogger asserts at compile time that *Logger satisfies
// botlib.Logger, then exercises each level for a smoke check.
func TestImplementsLogger(t *testing.T) {
	var _ botlib.Logger = (*Logger)(nil)

	l, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Debugf("debug %d", 1)
	l.Infof("info")
	l.Warnf("warn %s", "x")
	l.Errorf("error")
	_ = l.Sync() // stderr Sync may error on some platforms; ignore
}

func TestDevModeBuilds(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	l, err := New()
	if err != nil {
		t.Fatalf("New (dev): %v", err)
	}
	l.Infof("dev mode")
}
