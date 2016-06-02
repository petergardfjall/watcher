package engine

import (
	"github.com/petergardfjall/watcher/config"
	"github.com/petergardfjall/watcher/ping"
	"bytes"
	"fmt"
	"sync"
	"time"
)

// A StatusUpdate is sent by a PingerTask to its status channel for every
// execution of its Pinger to notify interested parties of the Pinger's status.
type StatusUpdate struct {
	Name   string
	Status PingerTaskStatus
}

// PingerTaskStatus describes the current status of a PingerTask.
type PingerTaskStatus struct {
	// Most recent ping result.
	LatestResult ping.Result
	// The number of consecutive pings that have resulted in the most
	// recent ping result.
	Consecutive int
	// Time of last successful ping (or nil if none has been successful).
	LatestOK *time.Time
	// Time of last unsuccessful ping (or nil if none has failed).
	LatestNOK *time.Time
}

// A PingerTask is responsible for periodically executing a given Pinger and
// pushing the ping result as a StatusUpdate on its status channel.
type PingerTask struct {
	Name     string
	Type     string
	Pinger   ping.Pinger
	Schedule config.Schedule
	// Engine WaitGroup that PingerTask will notify when done.
	WaitGroup sync.WaitGroup

	// Current task status
	Status PingerTaskStatus
	// Latest output returned by pinger
	Output *bytes.Buffer

	// statusUpdateChannel is a write-only channel that the PingerTask
	// sends PingerStatusUpdates on.
	statusChan chan<- StatusUpdate
}

//
// PingerTask methods
//

// Start starts the execution of the PingerTask. It will execute the Pinger
// according to the given schedule and post StatusUpdates on its status channel.
func (task *PingerTask) Start() {
	// signal to Engine when we're done
	defer task.WaitGroup.Done()

	task.Status = PingerTaskStatus{
		LatestResult: ping.Result{
			Status: ping.StatusUnknown,
			Error:  fmt.Errorf("no ping performed yet")},
		Consecutive: 1,
	}

	delay := task.Schedule.Interval.Duration
	log.Infof("[%s] started. interval: %s. retries: %+v", task.Name, delay, *task.Schedule.Retries)
	for {
		log.Debugf("[%s] waiting %s before next run ...", task.Name, delay)
		time.Sleep(delay)
		log.Infof("[%s] pinging ...", task.Name)
		result, output := task.ping()
		log.Debugf("[%s] result: %s", task.Name, result)
		if output != nil {
			log.Debugf("[%s] output: %s", task.Name, output.String())
		}
		task.updateStatus(result, output)
		log.Infof("[%s] status: %+v", task.Name, task.Status)
	}

}

// ping performs a ping (with the configured number of attempts for the
// PingerTask)
func (task *PingerTask) ping() (result ping.Result, output *bytes.Buffer) {
	attemptDelay := task.Schedule.Retries.Delay.Duration
	maxAttempts := task.Schedule.Retries.Attempts
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Debugf("[%s] attempt %d ...", task.Name, attempt)
		result, output = task.Pinger.Ping()
		log.Debugf("[%s] attempt %d result: %s", task.Name, attempt, result)
		if result.Status == ping.StatusOK {
			return
		}
		// make new attempt (possibly with exponential backoff)
		if task.Schedule.Retries.ExponentialBackoff {
			attemptDelay = attemptDelay * 2
		}
		if attempt < maxAttempts {
			time.Sleep(attemptDelay)
		}
	}
	return
}

// updateStatus sets the status for the PingerTask and sends a
// PingerStatusUpdate on the statusUpdateChannel
func (task *PingerTask) updateStatus(result ping.Result, output *bytes.Buffer) {
	if result.Status == task.Status.LatestResult.Status {
		task.Status.Consecutive++
	} else {
		task.Status.Consecutive = 1
	}
	now := time.Now().UTC()
	switch result.Status {
	case ping.StatusOK:
		task.Status.LatestOK = &now
	case ping.StatusNOK:
		task.Status.LatestNOK = &now
	}
	task.Status.LatestResult = result
	task.Output = output

	task.statusChan <- StatusUpdate{Name: task.Name, Status: task.Status}
}
