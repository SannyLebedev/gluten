// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promise

import (
	"context"
	"sync"
	"testing"
	"time"
)

type IntPromise Promise

func NewIntPromise() *IntPromise {
	return (*IntPromise)(New())
}

func (p *IntPromise) Deliver(val int) {
	(*Promise)(p).Deliver(val)
}

func (p *IntPromise) Get(ctx context.Context) (int, error) {
	val, err := (*Promise)(p).Get(ctx)
	if err != nil {
		return 0, err
	}
	return val.(int), err
}

func TestSequentialReadWrite(t *testing.T) {
	p := NewIntPromise()
	p.Deliver(10)
	for i := 0; i < 10; i++ {
		val, err := p.Get(context.Background())
		if err != nil {
			t.Fatal(err.Error())
		}
		if val != 10 {
			t.Fatalf("Promise was %d, not 10", val)
		}
	}
}

func TestTimeout(t *testing.T) {
	p := NewIntPromise()
	ctx, _ := context.WithTimeout(context.Background(), 50*time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			_, err := p.Get(ctx)
			if err != context.DeadlineExceeded {
				t.Fatal(err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestEventualReturn(t *testing.T) {
	p := NewIntPromise()
	ctx, _ := context.WithTimeout(context.Background(), 50*time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			_, err := p.Get(ctx)
			if err != nil {
				t.Fatal(err.Error())
			}
			wg.Done()
		}()
	}
	time.Sleep(1 * time.Millisecond)
	p.Deliver(10)
	wg.Wait()
}
