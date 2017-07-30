// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncx

import (
	"errors"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/hypirion/gluten/iox"
)

const (
	suspendStateOpen = iota
	suspendStateSuspended
	suspendStateClosed
)

type dummySuspender struct {
	mut          sync.Mutex
	suspendState int
}

func (ds *dummySuspender) Suspend() error {
	ds.mut.Lock()
	defer ds.mut.Unlock()
	if ds.suspendState == suspendStateClosed {
		return errors.New("custom closed error message")
	}
	if ds.suspendState == suspendStateSuspended {
		return errors.New("already suspended")
	}
	ds.suspendState = suspendStateSuspended
	return nil
}

func (ds *dummySuspender) Resume() error {
	ds.mut.Lock()
	defer ds.mut.Unlock()
	if ds.suspendState == suspendStateClosed {
		return errors.New("custom closed error message")
	}
	if ds.suspendState == suspendStateOpen {
		return errors.New("already open")
	}
	ds.suspendState = suspendStateOpen
	return nil
}

func (ds *dummySuspender) Close() error {
	ds.mut.Lock()
	defer ds.mut.Unlock()
	if ds.suspendState == suspendStateClosed {
		return errors.New("custom closed error message")
	}
	ds.suspendState = suspendStateClosed
	return nil
}

func TestBasicSuspendLocker(t *testing.T) {
	ds := &dummySuspender{}
	sl := NewSuspendLocker(ds, nil)
	err := sl.Suspend()
	if err != nil {
		t.Fatal(err)
	}
	err = sl.RLock()
	if err != nil {
		t.Fatal(err)
	}
	sl.RUnlock()
	if ds.suspendState != suspendStateOpen {
		t.Fatalf("Expected suspender to be in state %d, but was in state %d", suspendStateOpen, ds.suspendState)
	}
	err = sl.Suspend()
	if err != nil {
		t.Fatal(err)
	}
	if ds.suspendState != suspendStateSuspended {
		t.Fatalf("Expected suspender to be in state %d, but was in state %d", suspendStateSuspended, ds.suspendState)
	}
	err = sl.Close()
	if err != nil {
		t.Fatal(err)
	}
	if ds.suspendState != suspendStateClosed {
		t.Fatalf("Expected suspender to be in state %d, but was in state %d", suspendStateClosed, ds.suspendState)
	}
	err = sl.Suspend()
	if !iox.IsErrClosed(err) {
		t.Fatalf("Expected SuspendLocker to handle closed state, but got error %#v back", err)
	}
	err = sl.Resume()
	if !iox.IsErrClosed(err) {
		t.Fatalf("Expected SuspendLocker to handle closed state, but got error %#v back", err)
	}
	err = sl.Close()
	if err != nil {
		t.Fatalf("Expected SuspendLocker to handle closed state, but got error %#v back", err)
	}
}

func TestSuspendLockerIdempotent(t *testing.T) {
	f := func(operators []bool) bool {
		slock := NewSuspendLocker(&dummySuspender{}, nil)
		var toplevelErr error
		var errMut sync.Mutex
		var wg sync.WaitGroup
		for _, operator := range operators {
			wg.Add(1)
			shouldResume := operator
			go func() {
				var err error
				if shouldResume {
					err = slock.Resume()
				} else {
					err = slock.Suspend()
				}
				if err != nil {
					errMut.Lock()
					toplevelErr = err
					errMut.Unlock()
				}
				wg.Done()
			}()
		}
		wg.Wait()
		return toplevelErr == nil
	}
	// Ahum, abuse of the quick package really, as f is not deterministic. But
	// since quick doesn't shrink, it shouldn't be that problematic.
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestAutoSuspendLocker(t *testing.T) {
	ds := &dummySuspender{}
	asl := NewSuspendLocker(ds, &SuspendLockerOpts{MaxIdleTime: 1 * time.Millisecond})
	for i := 0; i < 100; i++ {
		time.Sleep(300 * time.Nanosecond)
		err := asl.RLock()
		if err != nil {
			t.Fatal(err)
		}
		asl.RUnlock()
	}
	ds.mut.Lock()
	state := ds.suspendState
	ds.mut.Unlock()
	if state != suspendStateOpen {
		t.Fatalf("Unexpected suspend state: %d", state)
	}
}
