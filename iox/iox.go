// Copyright 2017 Jean Niklas L'orange.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package iox provides higher level interfaces to I/O resources.
package iox

import (
	"errors"
	"os"
)

// ErrClosed is a generalization of os.ErrClosed.
var ErrClosed = errors.New("resource is closed")

// IsErrClosed returns true if the error is ErrClosed or os.ErrClosed.
func IsErrClosed(err error) bool {
	return err == ErrClosed || err == os.ErrClosed
}

// Suspender is an interface implemented by types that can be temporarily
// suspended. You would usually like to make resources suspendable if they are –
// either collectively or singularly – consuming a significant amount of other
// resources while being unused.
type Suspender interface {
	// Close should close the resource, regardless of whether the resource is
	// suspended or resumed.
	Close() error
	// Suspend temporarily stops the resources and releases resources which is not
	// required while being idle. Behaviour when the resource is already suspended
	// is undefined, and should be specified by concrete implementations.
	Suspend() error
	// Resume resumes the resource if it is suspended. Behaviour when the resource
	// is not suspended is undefined, and should be specified by concrete
	// implementations.
	Resume() error
}
