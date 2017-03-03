// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package idempotent contains the type Idempotent, which can be used for time
// dependent idempotent tasks. Typically, this is used for eventually consistent
// work (Example: DNS), but could also be used to avoid doing unnecessary work
// when updates happen quickly (e.g. cache updates).
package idempotent

// Idempotent is a task runner designed for time dependent idempotent tasks: If
// it is okay to throw away some tasks, provided one one of the tasks will be
// ran after this task was posted, then this is a good fit. Typical use cases
// include eventually consistent requests (DNS), cache updates reading all input
// from the database.
//
// More technically: An idempotent task runner can do one of three actions,
// depending on the current statue of the runner:
//
//   1. No task running, no queue: Task is ran immediately
//   2. Task is running, no queue: Task is queued
//   3. Task is running, one task in queue: Task is dropped
//
// Whenever a task has finished running, a queued task will immediately run.
type Idempotent struct {
	queue       chan struct{}
	ready       chan struct{}
	initialised bool
}

// New creates a new idempotent task runner.
func New() (idem *Idempotent) {
	idem = new(Idempotent)
	idem.queue = make(chan struct{}, 1)
	idem.ready = make(chan struct{}, 1)
	idem.ready <- struct{}{}
	idem.initialised = true
	return idem
}

// RunSync runs the task f if there are no other tasks waiting to be run,
// blocking until it has finished. If there are other tasks waiting to be run,
// this does nothing. Returns true if the task has been run, false otherwise.
//
// You should generally use RunSync over RunEventually if f depends on some
// resources (e.g. SQL transactions) that has to be manually closed or managed
// in some other way outside of the task.
func (idem *Idempotent) RunSync(f func()) bool {
	if !idem.initialised {
		panic("Idempotent task runner not initialised")
	}

	select {
	case idem.queue <- struct{}{}:
	default:
		return false
	}
	<-idem.ready
	<-idem.queue
	defer func() {
		idem.ready <- struct{}{}
	}()
	f()
	return true
}

// RunEventually runs the task f eventually if there are no other tasks waiting
// to be run. If there are other tasks waiting to be run, this does nothing.
// Returns true if the task will be run eventually or straight away, false
// otherwise.
//
// f must not panic.
func (idem *Idempotent) RunEventually(f func()) bool {
	if !idem.initialised {
		panic("Idempotent task runner not initialised")
	}

	select {
	case idem.queue <- struct{}{}:
	default:
		return false
	}
	go func() {
		<-idem.ready
		<-idem.queue
		f()
		idem.ready <- struct{}{}
	}()
	return true
}
