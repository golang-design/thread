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

// TestThread_TerminateConcurrent guards against the "send on closed
// channel" panic that occurred when Terminate was called concurrently:
// several goroutines would each try to signal shutdown on the same
// channel, and the sends that lost the race to close() panicked.
//
// The barrier (start) releases all callers at once so more than one
// passes the pre-check before the worker closes the channel -- that is
// the window the old code crashed in. An unrecovered panic in any of
// these goroutines fails the test.
func TestThread_TerminateConcurrent(t *testing.T) {
	for range 200 {
		th := thread.New()

		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				th.Terminate()
			}()
		}
		close(start)
		wg.Wait()
	}
}

// TestThread_CallAfterTerminate guards against the TOCTOU hang: a Call
// racing with Terminate could observe a not-yet-terminated thread, then
// block forever waiting on a result the now-exited worker will never
// deliver. Every iteration must finish within the timeout.
func TestThread_CallAfterTerminate(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 300 {
			th := thread.New()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				th.Call(func() {})
			}()
			go func() {
				defer wg.Done()
				th.Terminate()
			}()
			wg.Wait()
		}
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Call racing with Terminate hung; worker exited without " +
			"delivering the result")
	}
}

// TestThread_CleanupStopsWorker verifies the runtime.AddCleanup safety
// net stops a thread's worker (and its locked OS thread) once the Thread
// is dropped without Terminate. The original code used a finalizer that
// could never fire -- the worker goroutine closed over the Thread and
// kept it reachable forever -- so every dropped Thread leaked a
// goroutine. Here we drop many and require the count to fall well below
// the number created.
func TestThread_CleanupStopsWorker(t *testing.T) {
	const n = 40

	before := runtime.NumGoroutine()
	func() {
		for range n {
			th := thread.New()
			th.Call(func() {}) // ensure the worker is up
			_ = th
		}
	}() // all threads are unreachable after this returns

	stopped := eventually(10*time.Second, func() bool {
		runtime.GC()
		// Allow a small slack for unrelated runtime goroutines; the
		// point is that nowhere near n workers are still alive.
		return runtime.NumGoroutine() < before+n/2
	})
	if !stopped {
		t.Fatalf("workers not reclaimed after Threads became unreachable: "+
			"created %d, before=%d now=%d", n, before, runtime.NumGoroutine())
	}
}

// eventually polls cond until it returns true or the timeout elapses.
func eventually(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}
