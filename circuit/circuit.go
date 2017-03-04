// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package circuit contains circuit breakers to detect and prevent unnecessary
// load on systems that may be partially or completely down.
//
// The typical use from this package is to use some kind of Breaker for
// each system you connect to. Using any Breaker will follow the same
// pattern:
//
//	breaker := circuit.NewBreaker(params)
//	// ...
//	if err := breaker.IsTripped(); err != nil {
//		// short-circuit and handle error in some way
//	}
//	res, err := performAction()
//	// do some way of computing the severity of an error, if any
//	responseType := severity(err)
//	if err := breaker.Register(responseType); err != nil {
//		pager.Alert(err)
//	}
//	return res, err
//
// How a circuit breaker detects that it is tripped or how it should untrip is
// up to the particular implementation: Read the documentation for each breaker
package circuit

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// IsErrTripped returns true if the error is of type ErrTripped.
func IsErrTripped(err error) bool {
	_, ok := err.(ErrTripped)
	return ok
}

// ErrTripped is the error which is returned if a circuit breaker trips. The
// error will contain information about which service that caused the trip.
type ErrTripped struct {
	ServiceName string
}

func (err ErrTripped) Error() string {
	return "Circuit breaker for " + err.ServiceName + " has been tripped"
}

// ResponseType is the type of response from the service called.
type ResponseType int

const (
	// Success is the response type for actions that happened without any
	// trouble.
	Success ResponseType = iota
	// Anomaly is a minor error which happened on an action. It should not by
	// itself be considered an outage of a service (although multiple should).
	Anomaly
	// Fatal is a critical error which happened on an action. Sending this
	// should tell the circuit breaker that the service has an outage.
	Fatal
)

// Breaker is the interface for any circuit breaker
type Breaker interface {
	// IsTripped returns ErrTripped if the Breaker is tripped; i.e. the
	// error rate is so high that you should not call the service. Otherwise it
	// should return nil.
	IsTripped() error
	// Register should be called on the circuit breaker after any action on the
	// service/unit that the circuit breaker handles. This method returns
	// ErrTripped if this particular action changed the state of the circuit
	// breaker. You can ignore this error if you would like to, but typically you
	// should alert a pager of some kind that the circuit breaker tripped.
	Register(ResponseType) error
}

// CountBreakerParams are the parameters used to create a count breaker.
type CountBreakerParams struct {
	// MaxAnomalies is the maximal amount of anomalies the count breaker is
	// permitted to detect within the time window before it trips. A fatal
	// response is also considered an anomaly.
	MaxAnomalies uint32
	// MaxFatalities is the maximal amount of anomalies the count breaker is
	// permitted to detect within the time window before it trips.
	MaxFatalities uint32
	// TimeWindow is the length of the time window before the time. If unset, the
	// value is set to one minute.
	TimeWindow time.Duration
	// BackoffDuration is the duration the breaker will wait before it is
	// untripped. If unset, the value is set to one minute.
	BackoffDuration time.Duration
	// MaxBackoff is the maximal duration the breaker will wait before
	// untripping. If unset, the value is set to four minutes.
	MaxBackoff time.Duration
}

// NewCountBreaker creates a new CountBreaker.
func NewCountBreaker(serviceName string, params CountBreakerParams) *CountBreaker {
	if params.TimeWindow == 0 {
		params.TimeWindow = 1 * time.Minute
	}
	if params.BackoffDuration == 0 {
		params.BackoffDuration = 1 * time.Minute
	}
	if params.MaxBackoff == 0 {
		params.MaxBackoff = 4 * time.Minute
	}
	breaker := &CountBreaker{serviceName: serviceName, params: params}
	breaker.resetTime.Store(time.Now().Add(breaker.params.TimeWindow))
	return breaker
}

const (
	stateOpen = iota
	stateHalfOpen
	stateClosed
)

// CountBreaker is a circuit breaker that counts the amount of anomalies and
// fatalities within a specified time window. If the amount of
// anomalies/fatalities go over the specified threshold within a single time
// window, the count breaker will trip and cause an outage for at least
// BackoffDuration time (see CountBreakerParams). When the duration's up, the
// count breaker will be in a half-open state. If the first request(s) within a
// half-open state are anomalies/fatalities, the count breaker will immediately
// trip and wait with a randomized exponential timeout (up to MaxBackoff).
//
// The timewindow is not rolling: If you receive 4 anomalies in the last 5
// seconds of a time window, the anomaly count will still be reset to 0 when the
// time window is reset.
type CountBreaker struct {
	numAnomalies       uint32
	numFatalities      uint32
	resetTime          atomic.Value
	successiveFailures uint
	state              uint32
	serviceName        string
	mutex              sync.Mutex
	params             CountBreakerParams
}

func (c *CountBreaker) maybeReset() {
	resetTime := c.resetTime.Load().(time.Time)
	now := time.Now()
	if resetTime.Before(now) {
		c.mutex.Lock() // To ensure only one call resets the breaker
		// Has someone else reset the breaker while we waited for the lock? If so,
		// just bail out.
		updated := c.resetTime.Load().(time.Time)
		if updated != resetTime {
			c.mutex.Unlock()
			return
		}
		c.resetTime.Store(now.Add(c.params.TimeWindow))
		state := c.state
		// We might leak some requests here, but that should be fine on the edge of
		// a time window.
		atomic.StoreUint32(&c.numAnomalies, 0)
		atomic.StoreUint32(&c.numFatalities, 0)
		switch state {
		case stateOpen, stateHalfOpen:
			atomic.StoreUint32(&c.state, stateOpen)
			c.successiveFailures = 0
		case stateClosed:
			atomic.StoreUint32(&c.state, stateHalfOpen)
		}

		c.mutex.Unlock()
	}
}

func (c *CountBreaker) trip() bool {
	// trip may race with maybeReset for the lock, in which case, trip may
	// overwrite the reset. This shouldn't be an issue, as this will only happen
	// if the trip happens at the very end of a time window, in which case it's
	// alright to let trip take over.
	c.mutex.Lock()
	// Has someone else tripped the breaker while we waited for the lock? If so,
	// just bail out.
	if c.state == stateClosed {
		c.mutex.Unlock()
		return false
	}
	atomic.StoreUint32(&c.state, stateClosed)
	// Exponential backoff with randomization to avoid a thundering herd
	minTime := c.params.BackoffDuration << c.successiveFailures
	maxTime := c.params.BackoffDuration << (c.successiveFailures + 1)
	extraTime := time.Duration(rand.Int63n(int64(maxTime - minTime)))
	totalTime := minTime + extraTime
	if c.params.MaxBackoff <= totalTime {
		totalTime = c.params.MaxBackoff
	}
	c.resetTime.Store(time.Now().Add(totalTime))
	c.successiveFailures++
	c.mutex.Unlock()
	return true
}

// IsTripped returns an ErrTripped error iff the circuit breaker is tripped.
func (c *CountBreaker) IsTripped() error {
	c.maybeReset()
	state := atomic.LoadUint32(&c.state)
	switch state {
	case stateOpen, stateHalfOpen:
		return nil
	case stateClosed:
		return ErrTripped{c.serviceName}
	}
	panic("Implementation error in CountBreaker")
}

// Register registers the response type of an action. If this particular
// response causes a trip, the count breaker will return an ErrTripped error.
func (c *CountBreaker) Register(r ResponseType) error {
	c.maybeReset()
	state := atomic.LoadUint32(&c.state)
	switch r {
	case Success:
		if state == stateHalfOpen { // Assume the service is back up again
			atomic.StoreUint32(&c.state, stateOpen)
			// ... but note that we don't reset successive failures. If we end up
			// tripping in this time window, we will still consider it a successive
			// failure from last trip.
		}
	case Anomaly:
		prevAnomalies := atomic.AddUint32(&c.numAnomalies, 1) - 1
		// Exact match to avoid multiple trips, as that would cause lock contention
		if c.params.MaxAnomalies == prevAnomalies || state == stateHalfOpen {
			if c.trip() {
				return ErrTripped{c.serviceName}
			}
		}
	case Fatal:
		prevAnomalies := atomic.AddUint32(&c.numAnomalies, 1) - 1
		prevFatalities := atomic.AddUint32(&c.numFatalities, 1) - 1
		// Exact match to avoid multiple error values, to avoid lock contention.
		// Since we may trip on both anomalies and fatalities, we also check the
		// return value of trip, which will guarantee only one error.
		if c.params.MaxFatalities == prevFatalities || c.params.MaxAnomalies == prevAnomalies || state == stateHalfOpen {
			if c.trip() {
				return ErrTripped{c.serviceName}
			}
		}
	default:
		panic("Unknown response type")
	}
	return nil
}
