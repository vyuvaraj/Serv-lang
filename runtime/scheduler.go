package runtime

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

var (
	cronInstance *cron.Cron
	cronOnce     sync.Once
)

// Scheduler: Every
func Every(intervalStr string, callback func()) {
	duration, err := time.ParseDuration(intervalStr)
	if err != nil {
		if secs, err2 := fmt.Sscanf(intervalStr, "%d", &duration); err2 == nil && secs > 0 {
			duration = duration * time.Second
		} else {
			LogError("Invalid interval: ", intervalStr, " error: ", err)
			return
		}
	}

	go func() {
		ticker := time.NewTicker(duration)
		for range ticker.C {
			start := time.Now()
			MetricInc("scheduler_jobs_executed_total")
			endSpan := TraceScheduler("Every", intervalStr)
			func() {
				defer func() {
					if r := recover(); r != nil {
						MetricInc("scheduler_jobs_failed_total")
						LogError("Recovered in every job: ", r)
					}
				}()
				callback()
			}()
			endSpan()
			durationSecs := time.Since(start).Seconds()
			MetricGauge("scheduler_job_duration_seconds", durationSecs)
		}
	}()
}

// Scheduler: Cron
func Cron(cronExpr string, callback func()) {
	cronOnce.Do(func() {
		cronInstance = cron.New(cron.WithSeconds())
		cronInstance.Start()
	})

	_, err := cronInstance.AddFunc(cronExpr, func() {
		start := time.Now()
		MetricInc("scheduler_jobs_executed_total")
		endSpan := TraceScheduler("Cron", cronExpr)
		func() {
			defer func() {
				if r := recover(); r != nil {
					MetricInc("scheduler_jobs_failed_total")
					LogError("Recovered in cron job: ", r)
				}
			}()
			callback()
		}()
		endSpan()
		durationSecs := time.Since(start).Seconds()
		MetricGauge("scheduler_job_duration_seconds", durationSecs)
	})
	if err != nil {
		LogError("Failed to register cron expression: ", cronExpr, " error: ", err)
	}
}

// Sleep pauses execution for the given number of milliseconds.
func Sleep(ms interface{}) interface{} {
	var val int
	switch v := ms.(type) {
	case int:
		val = v
	case int64:
		val = int(v)
	case float64:
		val = int(v)
	case string:
		val, _ = strconv.Atoi(v)
	default:
		val, _ = strconv.Atoi(fmt.Sprint(v))
	}
	time.Sleep(time.Duration(val) * time.Millisecond)
	return nil
}

// CronNext computes the next execution time for a cron expression.
// Returns Unix timestamp (seconds) of the next occurrence.
// Usage: let nextTime = cron.next("0 */30 * * *")
func CronNext(cronExpr interface{}) interface{} {
	expr := fmt.Sprint(cronExpr)
	fields := strings.Fields(expr)
	var schedule cron.Schedule
	var err error

	if len(fields) == 6 {
		parser6 := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser6.Parse(expr)
	} else {
		parser5 := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser5.Parse(expr)
	}

	if err != nil {
		LogError("CronNext: invalid cron expression '", expr, "': ", err.Error())
		return 0
	}
	next := schedule.Next(time.Now())
	return next.Unix()
}

// CronSleepUntilNext sleeps until the next occurrence of the cron expression.
// Returns the Unix timestamp when it woke up.
// Supports both 5-field (min hour dom month dow) and 6-field (sec min hour dom month dow).
// Usage: cron.sleepUntilNext("0 */30 * * *")
func CronSleepUntilNext(cronExpr interface{}) interface{} {
	expr := fmt.Sprint(cronExpr)

	// Count fields to determine format
	fields := strings.Fields(expr)
	var schedule cron.Schedule
	var err error

	if len(fields) == 6 {
		// 6-field: second minute hour dom month dow
		parser6 := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser6.Parse(expr)
	} else {
		// 5-field: minute hour dom month dow
		parser5 := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser5.Parse(expr)
	}

	if err != nil {
		LogError("CronSleepUntilNext: invalid cron expression '", expr, "': ", err.Error())
		time.Sleep(60 * time.Second)
		return time.Now().Unix()
	}

	next := schedule.Next(time.Now())
	sleepDuration := time.Until(next)
	LogDebug("CronSleepUntilNext: expr='", expr, "' next=", next.Format(time.RFC3339), " sleeping ", sleepDuration.String())
	if sleepDuration > 0 {
		time.Sleep(sleepDuration)
	}
	return time.Now().Unix()
}

// SpawnWithTimeout runs a function with a timeout. Returns result or nil on timeout.
func SpawnWithTimeout(timeoutMs interface{}, fn func() interface{}) interface{} {
	timeout := time.Duration(toInt(timeoutMs)) * time.Millisecond
	ch := make(chan interface{}, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- nil
			}
		}()
		ch <- fn()
	}()
	select {
	case result := <-ch:
		return result
	case <-time.After(timeout):
		return nil
	}
}
