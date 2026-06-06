// Copyright 2026 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

package thread_test

import (
	"sync"
	"testing"

	"golang.design/x/thread"
)

func TestNewPoolPanicsOnNonPositive(t *testing.T) {
	for _, n := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("NewPool(%d) did not panic", n)
				}
			}()
			thread.NewPool(n)
		}()
	}
}

func TestPoolCallAndEval(t *testing.T) {
	p := thread.NewPool(3)
	defer p.Terminate()

	var got int
	if ok := p.Call(func() { got = 7 }); !ok || got != 7 {
		t.Fatalf("Call ran=%v got=%d; want true, 7", ok, got)
	}

	v, ok := thread.EvalPool(p, func() int { return 42 })
	if !ok || v != 42 {
		t.Fatalf("EvalPool = %d, %v; want 42, true", v, ok)
	}
}

func TestPoolNilFnIsNoop(t *testing.T) {
	p := thread.NewPool(2)
	defer p.Terminate()
	if p.Go(nil) || p.Call(nil) {
		t.Fatal("submitting a nil fn should report false")
	}
}

func TestPoolTerminateIsIdempotentAndRejects(t *testing.T) {
	p := thread.NewPool(2)
	p.Terminate()
	p.Terminate() // must not panic

	if p.Go(func() {}) {
		t.Fatal("Go after Terminate should return false")
	}
	if p.Call(func() {}) {
		t.Fatal("Call after Terminate should return false")
	}
	if _, ok := thread.EvalPool(p, func() int { return 1 }); ok {
		t.Fatal("EvalPool after Terminate should return ok=false")
	}
}

func TestPoolConcurrentSubmitters(t *testing.T) {
	p := thread.NewPool(4)
	defer p.Terminate()

	const n = 1000
	var mu sync.Mutex
	sum := 0

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			p.Call(func() {
				mu.Lock()
				sum++
				mu.Unlock()
			})
		}()
	}
	wg.Wait()

	if sum != n {
		t.Fatalf("ran %d tasks, want %d", sum, n)
	}
}
