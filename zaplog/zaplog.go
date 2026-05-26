// Package zaplog provides a zap-backed implementation of botlib.Logger.
// It lives in its own subpackage so bots that don't want zap don't pull
// go.uber.org/zap into their go.sum — only consumers that import
// botlib/zaplog get the dependency.
//
// The configuration mirrors roster's internal/logging: console encoder,
// ISO8601 timestamps, info level by default, debug + colorized level when
// APP_ENV=development.
package zaplog

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a zap-backed botlib.Logger. It also exposes Sync() so callers
// can flush on shutdown (defer l.Sync()).
type Logger struct {
	sugar *zap.SugaredLogger
}

// New builds a zap logger configured like roster's logging.New and wraps
// it as a botlib.Logger. The returned *Logger satisfies the
// botlib.Logger interface (Debugf/Infof/Warnf/Errorf).
func New() (*Logger, error) {
	z, err := build()
	if err != nil {
		return nil, err
	}
	return &Logger{sugar: z.Sugar()}, nil
}

func build() (*zap.Logger, error) {
	isDev := strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "development")

	cfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       isDev,
		Encoding:          "console",
		OutputPaths:       []string{"stderr"},
		ErrorOutputPaths:  []string{"stderr"},
		DisableCaller:     true,
		DisableStacktrace: true,
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:          "time",
			LevelKey:         "level",
			NameKey:          "logger",
			CallerKey:        "",
			FunctionKey:      "",
			MessageKey:       "msg",
			StacktraceKey:    "",
			LineEnding:       zapcore.DefaultLineEnding,
			EncodeLevel:      zapcore.CapitalLevelEncoder,
			EncodeTime:       zapcore.ISO8601TimeEncoder,
			EncodeDuration:   zapcore.StringDurationEncoder,
			EncodeName:       zapcore.FullNameEncoder,
			ConsoleSeparator: " ",
		},
	}
	if isDev {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	return cfg.Build()
}

func (l *Logger) Debugf(format string, args ...any) { l.sugar.Debugf(format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.sugar.Infof(format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.sugar.Warnf(format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.sugar.Errorf(format, args...) }

// Sync flushes buffered log entries. Call on shutdown (defer l.Sync()).
func (l *Logger) Sync() error { return l.sugar.Sync() }
