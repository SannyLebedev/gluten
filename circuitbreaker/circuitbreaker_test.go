// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package circuitbreaker

import (
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"testing/quick"
	"time"
)

type SmallUint32 uint32

func (u SmallUint32) Generate(rand *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(SmallUint32(rand.Uint32() % 1000))
}

// Bleh, we can probably make this prettier.
func TestConcurrentTrip(t *testing.T) {
	f := func(ok, bad SmallUint32) bool {
		breaker := NewCountBreaker("test", CountBreakerParams{MaxAnomalies: uint32(bad)})
		var wg sync.WaitGroup
		var startPistol sync.WaitGroup
		startPistol.Add(1)
		var lock sync.Mutex
		hasTripped := false
		good := true
		for i := 0; i < int(ok); i++ {
			wg.Add(1)
			go func() {
				startPistol.Wait()
				if err := breaker.IsTripped(); err == nil {
					err = breaker.Register(Success)
					if err != nil {
						t.Error("Got error from breaker, even though the call was successful")
						lock.Lock()
						good = false
						lock.Unlock()
					}
				}
				wg.Done()
			}()
		}
		for i := 0; i < int(bad)+1; i++ {
			wg.Add(1)
			go func() {
				startPistol.Wait()
				if err := breaker.IsTripped(); err != nil {
					t.Error("Breaker broke before all anomalies were registered")
				} else {
					err := breaker.Register(Anomaly)
					if err != nil {
						lock.Lock()
						if hasTripped {
							t.Error("Tripped more than once")
							good = false
						}
						hasTripped = true
						lock.Unlock()
					}
				}
				wg.Done()
			}()
		}
		startPistol.Done()
		wg.Wait()
		return hasTripped && good
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestSingleTrip(t *testing.T) {
	f := func(success, anomaly, fatal SmallUint32) bool {
		params := CountBreakerParams{
			MaxAnomalies:  uint32(anomaly) - 10,
			MaxFatalities: uint32(fatal) - 10,
		}
		breaker := NewCountBreaker("test", params)
		var wg sync.WaitGroup
		var startPistol sync.WaitGroup
		startPistol.Add(1)
		var lock sync.Mutex
		hasTripped := false
		good := true
		for i := 0; i < int(success); i++ {
			wg.Add(1)
			go func() {
				startPistol.Wait()
				if err := breaker.IsTripped(); err == nil {
					err = breaker.Register(Success)
					if err != nil {
						t.Error("Got error from breaker, even though the call was successful")
						lock.Lock()
						good = false
						lock.Unlock()
					}
				}
				wg.Done()
			}()
		}
		for i := 0; i < int(anomaly); i++ {
			wg.Add(1)
			go func() {
				startPistol.Wait()
				if err := breaker.IsTripped(); err == nil {
					err = breaker.Register(Anomaly)
					if err != nil {
						lock.Lock()
						if hasTripped {
							t.Error("Tripped more than once")
							good = false
						}
						hasTripped = true
						lock.Unlock()
					}
				}
				wg.Done()
			}()
		}
		for i := 0; i < int(fatal); i++ {
			wg.Add(1)
			go func() {
				startPistol.Wait()
				if err := breaker.IsTripped(); err == nil {
					err = breaker.Register(Fatal)
					if err != nil {
						lock.Lock()
						if hasTripped {
							t.Error("Tripped more than once")
							good = false
						}
						hasTripped = true
						lock.Unlock()
					}
				}
				wg.Done()
			}()
		}
		startPistol.Done()
		wg.Wait()
		return hasTripped && good
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestHalfOpen(t *testing.T) {
	params := CountBreakerParams{
		MaxAnomalies: 1,
		MaxBackoff:   1 * time.Millisecond,
	}
	breaker := NewCountBreaker("test", params)
	if breaker.Register(Anomaly) != nil {
		t.Error("Breaker immediately tripped")
	}
	if !IsErrTripped(breaker.Register(Anomaly)) {
		t.Error("Expected breaker to trip after second anomaly")
	}
	time.Sleep(2 * time.Millisecond)
	if breaker.IsTripped() != nil {
		t.Error("Expected breaker to be untripped")
	}
	if breaker.state != stateHalfOpen {
		t.Error("Expected breaker to be half-open")
	}
	if !IsErrTripped(breaker.Register(Anomaly)) {
		t.Error("Expected breaker to immedately trip from half-open state")
	}
	time.Sleep(2 * time.Millisecond)
	if breaker.IsTripped() != nil {
		t.Error("Expected breaker to be untripped")
	}
	if breaker.state != stateHalfOpen {
		t.Error("Expected breaker to be half-open")
	}
	if breaker.Register(Success) != nil {
		t.Error("Breaker shouldn't trip on success")
	}
	if breaker.state != stateOpen {
		t.Error("Expected breaker to be open")
	}
	if breaker.Register(Success) != nil {
		t.Error("Breaker shouldn't trip on first anomaly")
	}
	if breaker.state != stateOpen {
		t.Error("Expected breaker to be open")
	}
}
