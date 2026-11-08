package botlib

import (
	"context"

	"github.com/redpotatolabs/beacon/pkg/beacon"
)

// BeaconReporter wraps a beacon.Client with the standard log-then-post
// pattern used by tater and vault. Nil-safe: a zero-value Reporter just
// logs.
type BeaconReporter struct {
	Client *beacon.Client
	Logger Logger
}

// Report logs msg via the Logger and, if Client is set, posts msg to
// beacon. Beacon failures are logged but never propagated — observability
// must not break the caller's main flow.
func (r *BeaconReporter) Report(ctx context.Context, level, msg string, fields ...beacon.FieldKV) {
	logger := r.logger()
	logger.Infof("%s", msg)
	if r == nil || r.Client == nil {
		return
	}
	if _, err := r.Client.Error(ctx, beacon.ErrorRequest{
		Level:   level,
		Message: msg,
		Fields:  fields,
	}); err != nil {
		logger.Warnf("beacon: post failed: %v", err)
	}
}

func (r *BeaconReporter) logger() Logger {
	if r != nil && r.Logger != nil {
		return r.Logger
	}
	return StdLogger()
}
