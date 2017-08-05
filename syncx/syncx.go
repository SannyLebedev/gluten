// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package syncx implements several utility synchronization mechanisms not
// included in sync.
package syncx

import (
	"io"
	"sync"
	"time"

	"github.com/hyPiRion/gluten/iox"
)

// CloseLocker is a read-write lock on top of a closable resource. All
// implementations from this package provide an idempotent, threadsafe io.Closer
// interface on top of the resource, along with the read-write lock.
//
// CloseLockers are typically used inside other types which wrap a closeable
// resource.
type CloseLocker interface {
	io.Closer
	// Lock acquires write lock on this locker.
	Lock()
	// Unlock releases the write lock on this locker.
	Unlock()
	// RLock attempts to acquire a read lock. If the resource is closed, then this
	// call returns an error and does not acquire a read lock on the resource.
	RLock() error
	// RUnlock releases a read lock on this locker.
	RUnlock()
}

type rawCloseLocker struct {
	mut      sync.RWMutex
	closed   bool
	resource io.Closer
}

func (rcl *rawCloseLocker) Close() error {
	rcl.Lock()
	defer rcl.Unlock()
	if rcl.closed {
		return nil
	}
	err := rcl.resource.Close()
	if err == nil {
		rcl.closed = true
	}
	return err
}

func (rcl *rawCloseLocker) Lock() {
	rcl.mut.Lock()
}

func (rcl *rawCloseLocker) Unlock() {
	rcl.mut.Unlock()
}

func (rcl *rawCloseLocker) RLock() error {
	rcl.mut.RLock()
	if rcl.closed {
		rcl.mut.RUnlock()
		return iox.ErrClosed
	}
	return nil
}

func (rcl *rawCloseLocker) RUnlock() {
	rcl.mut.RUnlock()
}

// NewCloseLocker returns a new CloserLocker over c.
func NewCloseLocker(c io.Closer) CloseLocker {
	return &rawCloseLocker{resource: c}
}

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

// SuspendLockerOpts is a struct different options you can provide while
// creating a SuspendLocker. You can provide nil if you want the default
// behaviour.
type SuspendLockerOpts struct {
	// When AlreadySuspended is set, the suspend locker assumes the Suspender
	// is already in a suspended mode.
	AlreadySuspended bool
	// If set, MaxIdleTime will be the maximal time the Suspender will be open
	// after a call to [R]Lock.
	MaxIdleTime time.Duration
}

// NewSuspendLocker returns a SuspendLocker over s.
func NewSuspendLocker(s iox.Suspender, slo *SuspendLockerOpts) SuspendLocker {
	if slo == nil {
		slo = &SuspendLockerOpts{}
	}
	if slo.MaxIdleTime != 0 {
		return newAutoSuspendLocker(s, slo)
	}
	return newSuspendLocker(s, slo)
}

func newSuspendLocker(s iox.Suspender, slo *SuspendLockerOpts) *rawSuspendLocker {
	return &rawSuspendLocker{
		resource:  s,
		suspended: slo.AlreadySuspended,
	}
}

// TODO: Embed CloseLocker inside this one? Or not? Duplication of the resouce
// itself, but that's just a pointer. Hmmm.
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
		return nil
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

func newAutoSuspendLocker(s iox.Suspender, slo *SuspendLockerOpts) SuspendLocker {
	locker := newSuspendLocker(s, slo)
	trySuspend := func() { locker.Suspend() }
	return &autoSuspendLocker{
		rawSuspendLocker: locker,
		maxIdle:          slo.MaxIdleTime,
		timer:            time.AfterFunc(slo.MaxIdleTime, trySuspend),
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
