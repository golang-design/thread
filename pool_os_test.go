// Copyright 2026 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

//go:build linux

package thread_test

import (
	"sync"
	"testing"

	"golang.design/x/thread"
	"golang.org/x/sys/unix"
)

// TestPoolBoundsOSThreads is the headline property: all work submitted to the
// pool runs on at most Size() OS threads, regardless of how many tasks are
// submitted. This is the constructive thread cap that replaces reactively
// reaping surplus threads.
func TestPoolBoundsOSThreads(t *testing.T) {
	const size = 4
	p := thread.NewPool(size)
	defer p.Terminate()

	var mu sync.Mutex
	tids := map[int]bool{}

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Call(func() {
				mu.Lock()
				tids[unix.Gettid()] = true
				mu.Unlock()
			})
		}()
	}
	wg.Wait()

	if len(tids) > size {
		t.Fatalf("work ran on %d OS threads, want <= %d", len(tids), size)
	}
}

// TestPoolUsesAllThreads occupies every pool thread simultaneously and checks
// that the tasks landed on exactly Size() distinct OS threads — confirming the
// pool actually spreads work, not just that it caps it.
func TestPoolUsesAllThreads(t *testing.T) {
	const size = 4
	p := thread.NewPool(size)
	defer p.Terminate()

	var mu sync.Mutex
	tids := map[int]bool{}

	start := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(size)
	done.Add(size)

	for i := 0; i < size; i++ {
		go func() {
			defer done.Done()
			p.Call(func() {
				mu.Lock()
				tids[unix.Gettid()] = true
				mu.Unlock()
				ready.Done()
				<-start // hold the thread so each task forces a different one
			})
		}()
	}

	ready.Wait() // every pool thread is now occupied
	close(start)
	done.Wait()

	if len(tids) != size {
		t.Fatalf("used %d distinct OS threads, want %d", len(tids), size)
	}
}
