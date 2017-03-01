// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package idempotent

import (
	"testing"
	"time"
)

type intVal struct {
	val int
}

func (e *intVal) slowSet(v int) func() {
	return func() {
		time.Sleep(100 * time.Millisecond)
		e.val = v
	}
}

func (e *intVal) fastSet(v int) (func(), <-chan struct{}) {
	ch := make(chan struct{}, 1)
	return func() {
		time.Sleep(50 * time.Millisecond)
		e.val = v
		ch <- struct{}{}
	}, ch
}

func TestIdempotentWait1(t *testing.T) {
	var val intVal
	idem := New()

	ran := idem.RunSync(val.slowSet(1))
	if !ran {
		t.Error("RunSync should've been ran")
	}
	if val.val != 1 {
		t.Error("RunSync did not set the value to 1")
	}
}

func TestIdempotentWait2(t *testing.T) {
	var val intVal
	idem := New()

	// perform a non-idempotent operation, which, although is not correct usage,
	// will validate that the call to RunSync will run.
	idem.RunEventually(val.slowSet(1))
	time.Sleep(10 * time.Millisecond)
	ran := idem.RunSync(val.slowSet(2))
	if !ran {
		t.Error("RunSync should've been ran")
	}
	if val.val != 2 {
		t.Error("RunSync did not set the value to 2")
	}
}

func TestIdempotentWaitMany(t *testing.T) {
	var val intVal
	idem := New()
	for i := 0; i < 10; i++ {
		idem.RunEventually(val.slowSet(10))
	}

	ran := idem.RunSync(val.slowSet(10))
	if ran {
		t.Error("RunSync should not be ran with eventually waiting functions")
	}
}

func TestIdempotentEventually1(t *testing.T) {
	var val intVal
	idem := New()

	f, ch := val.fastSet(10)
	idem.RunEventually(f)
	select {
	case <-ch:
		if val.val != 10 {
			t.Error("Did not set value")
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("timed out")
	}
}

func TestIdempotentEventually2(t *testing.T) {
	var val intVal
	idem := New()

	idem.RunEventually(val.slowSet(5))
	time.Sleep(1 * time.Millisecond)
	f, ch := val.fastSet(10)
	willRun := idem.RunEventually(f)
	if !willRun {
		t.Error("Should eventually run")
	}
	select {
	case <-ch:
		if val.val != 10 {
			t.Error("Did not set value")
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("timed out")
	}
}

func TestIdempotentEventually3(t *testing.T) {
	var val intVal
	idem := New()

	f, ch := val.fastSet(10)
	willRun := idem.RunEventually(f)
	if !willRun {
		t.Error("Should eventually run")
	}
	select {
	case <-ch:
		t.Error("Should time out")
	default:
	}
}
