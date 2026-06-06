// Copyright 2020 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

package thread_test

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"golang.design/x/thread"
)

// TestThread_NilFn covers the nil-function guards in Call and Go.
func TestThread_NilFn(t *testing.T) {
	th := thread.New()
	defer th.Terminate()
	th.Call(nil)
	th.Go(nil)
}

// TestThread_GoAfterTerminate covers Go's leading "already terminated"
// branch: a completed Terminate must make Go return without enqueuing.
func TestThread_GoAfterTerminate(t *testing.T) {
	th := thread.New()
	th.Terminate()
	th.Go(func() { panic("must not run on a terminated thread") })
}

// TestThread_QuitWhileEnqueuing covers the enqueue-time quit branches of
// Call and Go: quit becomes ready while a send is blocked on a full
// channel buffer. The worker is parked inside a call so it never drains
// the buffer, the buffer is filled to capacity, then Terminate is called
// while a Call and a Go are blocked trying to enqueue.
func TestThread_QuitWhileEnqueuing(t *testing.T) {
	th := thread.New()

	started := make(chan struct{})
	block := make(chan struct{})
	th.Go(func() {
		close(started)
		<-block // occupy the worker so it stops draining fdCh
	})
	<-started

	// Fill the channel buffer to capacity (New uses GOMAXPROCS(0)).
	for range runtime.GOMAXPROCS(0) {
		th.Go(func() {})
	}

	// A Call and a Go now block trying to send into the full buffer.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); th.Call(func() {}) }()
	go func() { defer wg.Done(); th.Go(func() {}) }()

	time.Sleep(20 * time.Millisecond) // let both block on the send

	th.Terminate() // quit becomes ready; the blocked sends take the quit branch

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked Call/Go did not unblock after Terminate")
	}

	close(block) // release the worker
}
