// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package promise implements a promise type. This has nothing to do with
// JavaScript promises; You should consider this equivalent to Clojure's
// promises.
package promise

import (
	"context"
	"sync"
)

// Promise is a type embedding a value. The value is or will be computed at some
// point, and the promise type gives you the option to wait until the value is
// computed. A promise differs from a channel in that a promise can only be set
// once, and can will read out the value multiple times.
//
// Depending on you use case, it may be beneficial to wrap the Promise in your
// own type. One way to do this is like so:
//
//	type IntPromise Promise
//
//	func NewIntPromise() *IntPromise {
//		return (*IntPromise)(promise.New())
//	}
//
//	func (p *IntPromise) Deliver(val int) {
//		(*promise.Promise)(p).Deliver(val)
//	}
//
//	func (p *IntPromise) Get(ctx context.Context) (int, error) {
//		val, err := (*promise.Promise)(p).Get(ctx)
//		if err != nil {
//			return 0, err
//		}
//		return val.(int), err
//	}
type Promise struct {
	mutex    sync.Mutex
	assigned bool
	val      interface{}
	done     chan struct{}
}

// New creates a new promise.
func New() *Promise {
	p := new(Promise)
	p.done = make(chan struct{})
	return p
}

// Deliver assigns a value to the promise if it does not already have a value.
// If it has a value, then this does nothing.
func (p *Promise) Deliver(val interface{}) {
	if p.done == nil {
		panic("Promise not initialised")
	}
	p.mutex.Lock()
	if !p.assigned {
		p.assigned = true
		p.val = val
		close(p.done)
	}
	p.mutex.Unlock()
}

// Get returns the value within the promise. If the value is not yet set, then
// this will block until either the value is set or the context times out.
func (p *Promise) Get(ctx context.Context) (interface{}, error) {
	if p.done == nil {
		panic("Promise not initialised")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return p.val, nil
	}
}

// Realized returns true if this Promise has been realized, false otherwise.
func (p *Promise) Realized() bool {
	if p.done == nil {
		panic("Promise not initialised")
	}
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

/*

// Skip these for now: People should just use context.

// Done returns a channel that's closed when the promise is assigned.
func (p *Promise) Done() <-chan struct{} {
	if p.done == nil {
		panic("Promise not initialised")
	}
	return p.done
}

// Value returns the value within the promise. If the value is not yet set, then
// this function will block until the value has been assigned.
func (p *Promise) Value() interface{} {
	if p.done == nil {
		panic("Promise not initialised")
	}
	select {
	case <-p.done:
		return p.val
	}
}

*/
