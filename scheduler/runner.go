package scheduler

import (
	"errors"
	"os"
	"time"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . BuildScheduler

type BuildScheduler interface {
	Schedule(logger lager.Logger, interval time.Duration) error
	TriggerImmediately(logger lager.Logger, jobConfig atc.JobConfig, resourceConfigs atc.ResourceConfigs, resourceTypes atc.ResourceTypes) (db.Build, error)
}

var errPipelineRemoved = errors.New("pipeline removed")

type Runner struct {
	Logger lager.Logger

	Scheduler BuildScheduler

	Noop bool

	Interval time.Duration
}

// runs tick() on an interval
func (runner *Runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	if runner.Interval == 0 {
		panic("unconfigured scheduler interval")
	}

	runner.Logger.Info("start", lager.Data{
		"inverval": runner.Interval.String(),
	})

	defer runner.Logger.Info("done")

dance:
	for {
		err := runner.tick(runner.Logger.Session("tick"))
		if err != nil {
			return err
		}

		select {
		case <-time.After(runner.Interval):
		case <-signals:
			break dance
		}
	}

	return nil
}

// acquires a lease, loads versionsdb, calls schedule for each job
func (runner *Runner) tick(logger lager.Logger) error {
	if runner.Noop {
		return nil
	}

	return runner.Scheduler.Schedule(logger, runner.Interval)
}
