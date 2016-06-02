package engine

import (
	"github.com/petergardfjall/watcher/config"
	"github.com/petergardfjall/watcher/ping"
	"fmt"
	"github.com/op/go-logging"
	"sync"
	"time"
)

var log = logging.MustGetLogger("engine")

var (
	// default DefaultSchedule to use when no defaultSchedule is given
	// in EngineConfig
	defaultInterval         = config.Duration{Duration: 10 * time.Minute}
	defaultRetryDelay       = config.Duration{Duration: 3 * time.Second}
	standardDefaultSchedule = config.Schedule{
		Interval: &defaultInterval,
		Retries: &config.Retries{
			Attempts:           3,
			Delay:              defaultRetryDelay,
			ExponentialBackoff: false,
		},
	}
)

// An Engine drives the execution of a set of Pingers, each according to their
// configured schedule.
type Engine struct {
	Pingers         map[string]*PingerTask
	WaitGroup       sync.WaitGroup
	DefaultSchedule config.Schedule
}

//
// Engine functions and methods
//

// NewEngine creates a new Engine from a configuration.
func NewEngine(engineConf *config.Engine, advertisedBaseURL string) (engine *Engine, err error) {
	engine = new(Engine)

	if engineConf.DefaultSchedule != nil {
		engine.DefaultSchedule = *engineConf.DefaultSchedule
	} else {
		engine.DefaultSchedule = standardDefaultSchedule
	}

	// channel that PingerTasks will use to send PingerTaskStatuses
	// to alert.Dispatcher
	statusChannel := make(chan StatusUpdate, 100)

	engine.Pingers = make(map[string]*PingerTask)
	for _, pingerConf := range engineConf.Pingers {
		log.Debugf("instantiating %s pinger", pingerConf.Type)
		var pinger ping.Pinger
		switch pingerConf.Type {
		case "ssh":
			pinger, err = ping.NewSSHPinger(&pingerConf)
		case "http":
			pinger, err = ping.NewHTTPPinger(&pingerConf)

		default:
			err = fmt.Errorf("unknown pinger type: %s", pingerConf.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate pinger: %s", err)
		}

		// either set schedule given in pinger config or use default
		var pingerSchedule config.Schedule
		if pingerConf.Schedule != nil {
			pingerSchedule = *pingerConf.Schedule
		} else {
			pingerSchedule = engine.DefaultSchedule
		}
		engine.WaitGroup.Add(1)
		engine.Pingers[pingerConf.Name] = &PingerTask{
			Name:       pingerConf.Name,
			Type:       pingerConf.Type,
			Pinger:     pinger,
			Schedule:   pingerSchedule,
			WaitGroup:  engine.WaitGroup,
			statusChan: statusChannel}
	}

	dispatcher, err := NewDispatcher(engineConf.Alerter, statusChannel)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate alert dispatcher: %s", err)
	}
	go dispatcher.Start()

	return engine, nil
}

// Start activates the Engine, starting all configured Pingers.
func (engine *Engine) Start() {
	for _, pinger := range engine.Pingers {
		go pinger.Start()
	}
}

// Await awaits the completion of all Pingers
func (engine *Engine) Await() {
	engine.WaitGroup.Wait()
}
