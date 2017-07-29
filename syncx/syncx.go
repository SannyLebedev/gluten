// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package syncx implements several utility synchronization mechanisms not
// included in sync.
package syncx

import (
	"sync"
	"time"

	"github.com/hypirion/gluten/iox"
)

// SuspendLocker is a read-write lock on top of a suspendable resource. All
// implementations from this package provide an idempotent, threadsafe Suspender
// interface on top of the resource, along with a read-write lock.
//
// SuspendLockers are typically used inside other types which wrap a suspendable
// resource.
type SuspendLocker interface {
	iox.Suspender
	// Lock acquires write lock on this locker. Note that any iox.Suspender calls
	// will temporarily acquire the write lock by themselves, so the write lock is
	// only necessary if the underlying resource does not have threadsafe function
	// calls.
	Lock()
	// Unlock releases the write lock on this locker.
	Unlock()
	// RLock attempts to acquire a read lock. If the resource is suspended, it is
	// automatically resumed. If the resource is closed or resuming the resource
	// causes an error, then this call returns an error and does not acquire a
	// read lock on the resource.
	RLock() error
	// RUnlock releases a read lock on this locker.
	RUnlock()
}

// NewSuspendLocker returns a SuspendLocker.
func NewSuspendLocker(s iox.Suspender, suspended bool) SuspendLocker {
	return &rawSuspendLocker{
		resource:  s,
		suspended: suspended,
	}
}

type rawSuspendLocker struct {
	mut       sync.RWMutex
	closed    bool
	suspended bool
	resource  iox.Suspender
}

func (rsl *rawSuspendLocker) Close() error {
	rsl.Lock()
	defer rsl.Unlock()
	if rsl.closed {
		return iox.ErrClosed
	}
	err := rsl.resource.Close()
	if err == nil {
		rsl.closed = true
	}
	return err
}

func (rsl *rawSuspendLocker) Suspend() error {
	rsl.Lock()
	defer rsl.Unlock()
	if rsl.closed {
		return iox.ErrClosed
	}
	if rsl.suspended {
		return nil
	}
	err := rsl.resource.Suspend()
	if err == nil {
		rsl.suspended = true
	}
	return err
}

func (rsl *rawSuspendLocker) Resume() error {
	rsl.Lock()
	defer rsl.Unlock()
	if rsl.closed {
		return iox.ErrClosed
	}
	if !rsl.suspended {
		return nil
	}
	err := rsl.resource.Resume()
	if err == nil {
		rsl.suspended = false
	}
	return err
}

func (rsl *rawSuspendLocker) Lock() {
	rsl.mut.Lock()
}

func (rsl *rawSuspendLocker) Unlock() {
	rsl.mut.Unlock()
}

func (rsl *rawSuspendLocker) RLock() error {
	for {
		rsl.mut.RLock()
		if rsl.closed {
			rsl.mut.RUnlock()
			return iox.ErrClosed
		}
		if rsl.suspended {
			// We have to ensure we don't perform concurrent resumes, hence we unlock
			// the read lock. But once we release the write lock, someone may suspend
			// the resource. Hence the for loop here.
			rsl.mut.RUnlock()
			err := rsl.Resume()
			if err != nil {
				return err
			}
			continue
		}
		break
	}
	return nil
}

func (rsl *rawSuspendLocker) RUnlock() {
	rsl.mut.RUnlock()
}

// NewAutoSuspendLocker returns a SuspendLocker, which will automatically
// suspend the resource after an amount of idleness. Idleness is the duration
// since last time Lock or RLock was called on this SuspendLocker.
func NewAutoSuspendLocker(s iox.Suspender, suspended bool, maxIdle time.Duration) SuspendLocker {
	locker := NewSuspendLocker(s, suspended)
	trySuspend := func() { locker.Suspend() }
	return &autoSuspendLocker{
		rawSuspendLocker: locker.(*rawSuspendLocker),
		maxIdle:          maxIdle,
		timer:            time.AfterFunc(maxIdle, trySuspend),
	}
}

type autoSuspendLocker struct {
	*rawSuspendLocker
	maxIdle time.Duration
	timer   *time.Timer
}

func (asl *autoSuspendLocker) Close() error {
	asl.timer.Stop()
	return asl.rawSuspendLocker.Close()
}

func (asl *autoSuspendLocker) Lock() {
	asl.rawSuspendLocker.Lock()
	if !asl.rawSuspendLocker.closed {
		asl.timer.Reset(asl.maxIdle)
	}
}

func (asl *autoSuspendLocker) RLock() error {
	err := asl.rawSuspendLocker.RLock()
	if err != nil {
		return err
	}
	asl.timer.Reset(asl.maxIdle)
	return nil
}
