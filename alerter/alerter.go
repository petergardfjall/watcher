package alerter

import (
	"github.com/op/go-logging"
	"time"
)

var log = logging.MustGetLogger("alerter")

// PingerStatus describes the current status of a pinger.
// If the ping was successful, OK will be true and Error will be nil.
// Should the ping not have been successful, OK  will be false and
// Error will contain the  error that caused the ping to fail.
type PingerStatus struct {
	OK    bool
	Error string
	// A URL to the watcher where the latest output for the given pinger
	// can be found (if any).
	OutputURL string
}

// A PingerUpdate is sent to an Alerter from a Pinger to notify
// the Alerter of the result of a recent health check of the Pinger's
// endpoint.
type PingerUpdate struct {
	Name        string
	Status      PingerStatus
	Consecutive int
	LatestOK    *time.Time
	LatestNOK   *time.Time
}

// Alerter implmentations send notification messages over a given
// protocol (such as SMTP or HTTP) to a collection of interested
// receivers.
type Alerter interface {
	Alert(update PingerUpdate) error
}
