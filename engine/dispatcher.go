package engine

import (
	"github.com/petergardfjall/watcher/alerter"
	"github.com/petergardfjall/watcher/config"
	"github.com/petergardfjall/watcher/ping"
	"fmt"
	"time"
)

// A Dispatcher pushes pinger status updates to its set of configured Alerters.
type Dispatcher struct {
	statusChan        <-chan StatusUpdate
	alerters          []alerter.Alerter
	alertHistory      map[string]time.Time
	reminderDelay     time.Duration
	advertisedBaseURL string
}

// NewDispatcher creates a new Dispatcher with a set of Alerters as configured
// in an alertsConfig. The Dispatcher will listen for incoming Pinger status
// updates on a channel and push those updates to its set of configured
// Alerters.
func NewDispatcher(alertsConfig *config.Alerter,
	statusChan <-chan StatusUpdate) (*Dispatcher, error) {
	var alerters []alerter.Alerter

	if alertsConfig.Email != nil {
		log.Debugf("setting up email alerter ...")
		alerter, err := alerter.NewEmailAlerter(alertsConfig.Email)
		if err != nil {
			return nil, fmt.Errorf("dispatcher: failed to initialize email alerter: %s", err)
		}
		alerters = append(alerters, alerter)
	}

	alertHistory := make(map[string]time.Time)
	baseURL := fmt.Sprintf("https://%s:%d", alertsConfig.AdvertisedIP, alertsConfig.AdvertisedPort)
	return &Dispatcher{statusChan, alerters, alertHistory, alertsConfig.ReminderDelay.Duration, baseURL}, nil
}

// Start activates this Dispatcher, making it start listening for pinger status
// updates on its status channel.
func (dispatcher *Dispatcher) Start() {
	for {
		select {
		case statusUpdate := <-dispatcher.statusChan:
			if !dispatcher.shouldPublish(statusUpdate) {
				log.Debugf("suppressing: %+v", statusUpdate)
				continue
			}
			pingResult := statusUpdate.Status.LatestResult
			var error string
			if pingResult.Error != nil {
				error = pingResult.Error.Error()
			}
			status := alerter.PingerStatus{
				OK:        pingResult.Status == ping.StatusOK,
				Error:     error,
				OutputURL: outputURL(dispatcher.advertisedBaseURL, statusUpdate.Name),
			}

			update := alerter.PingerUpdate{
				Name:        statusUpdate.Name,
				Status:      status,
				Consecutive: statusUpdate.Status.Consecutive,
				LatestOK:    statusUpdate.Status.LatestOK,
				LatestNOK:   statusUpdate.Status.LatestNOK,
			}

			log.Debugf("dispatching %+v", statusUpdate)
			dispatcher.dispatch(update)
		}
	}

}

func (dispatcher *Dispatcher) dispatch(update alerter.PingerUpdate) {
	log.Infof("dispatching pinger update: %+v", update)

	for _, a := range dispatcher.alerters {
		go func(a alerter.Alerter) {
			if err := a.Alert(update); err != nil {
				log.Errorf("alert failed: %s", err)
			}
		}(a)
	}

	dispatcher.alertHistory[update.Name] = time.Now().UTC()
}

// shouldPublish returns true if a given status update warrants an alert.
// This is the case if a state transition has taken place for the pinger or
// if the pinger failed and the reminder delay has been exceeded since the
// last alert.
func (dispatcher *Dispatcher) shouldPublish(update StatusUpdate) bool {
	pingerName := update.Name
	// state transistions are always to be published
	if statusChanged(update.Status) {
		log.Debugf("state transition on [%s]", pingerName)
		return true
	}

	// if not a state transition, we only alert of error states in
	// case the reminder delay has passed since the last alert.
	if update.Status.LatestResult.Status == ping.StatusNOK {
		if lastAlert, ok := dispatcher.alertHistory[pingerName]; ok {
			timeUntilReminder := dispatcher.reminderDelay - time.Since(lastAlert)
			log.Debugf("time until reminder for [%s]: %s", pingerName, timeUntilReminder.String())
			return timeUntilReminder <= 0
		}
	}

	return false
}

// statusChanged returns true if a StatusUpdate conveys a state transition
// (for example, from StatusOK to StatusNOK) indicated by the Consecutive
// field being equal to 1. A pinger being in state unknown does not count
// as a state change either (it is the initial state of the pinger).
func statusChanged(status PingerTaskStatus) bool {
	return status.Consecutive == 1 && status.LatestResult.Status != ping.StatusUnknown
}

func outputURL(baseURL string, pingerName string) string {
	return fmt.Sprintf("%s/pingers/%s/output", baseURL, pingerName)
}
