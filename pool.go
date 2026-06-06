// Copyright 2026 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

package thread

import "sync"

// Pool is a bounded set of OS-locked threads that execute submitted work.
//
// It bounds the number of OS threads a workload consumes by construction:
// instead of letting blocking calls each spin up a fresh thread (and then
// reactively reaping the surplus), all work is funneled onto a fixed number of
// threads the pool owns. Idle threads pick up the next task, so work is
// balanced across the pool without per-task scheduling.
type Pool struct {
	tasks   chan func()
	quit    chan struct{}
	threads []Thread
	once    sync.Once
}

// NewPool creates a Pool backed by n threads. It panics if n <= 0.
func NewPool(n int) *Pool {
	if n <= 0 {
		panic("thread: NewPool requires n > 0")
	}
	p := &Pool{
		tasks:   make(chan func()),
		quit:    make(chan struct{}),
		threads: make([]Thread, n),
	}
	for i := range p.threads {
		th := New()
		// Each thread runs a single long-lived consumer loop that occupies
		// the thread's worker for the pool's lifetime. The loop returns only
		// when quit is closed, after which Terminate tears the thread down.
		th.Go(func() {
			for {
				// Give quit priority so a terminated pool discards queued but
				// unstarted tasks rather than racing to run one more.
				select {
				case <-p.quit:
					return
				default:
				}
				select {
				case <-p.quit:
					return
				case fn := <-p.tasks:
					fn()
				}
			}
		})
		p.threads[i] = th
	}
	return p
}

// Size returns the number of threads in the pool.
func (p *Pool) Size() int { return len(p.threads) }

// Go submits fn to run on some pool thread and returns true once a thread has
// accepted it. It blocks until an idle thread is available, providing
// backpressure that keeps in-flight work bounded by the pool size. It returns
// false without running fn if the pool was terminated first, or if fn is nil.
//
// A task that has been accepted always runs to completion, even if the pool is
// terminated meanwhile; only not-yet-accepted tasks are discarded.
func (p *Pool) Go(fn func()) bool {
	if fn == nil {
		return false
	}
	select {
	case <-p.quit:
		return false
	case p.tasks <- fn:
		return true
	}
}

// Call submits fn and blocks until it has run. It returns false without
// running fn if the pool was terminated before fn could be accepted, or if fn
// is nil.
func (p *Pool) Call(fn func()) bool {
	if fn == nil {
		return false
	}
	done := make(chan struct{})
	if !p.Go(func() {
		defer close(done)
		fn()
	}) {
		return false
	}
	<-done
	return true
}

// Terminate stops every thread in the pool and discards any not-yet-accepted
// tasks. It is safe to call multiple times, including concurrently.
func (p *Pool) Terminate() {
	p.once.Do(func() {
		// Close quit first so the consumer loops return, then terminate the
		// underlying threads to release their OS threads.
		close(p.quit)
		for _, t := range p.threads {
			t.Terminate()
		}
	})
}

// EvalPool runs fn on some pool thread and returns its result. It blocks until
// fn returns. If the pool was terminated before fn could be accepted, it
// returns the zero value of T and false.
//
// EvalPool is the typed counterpart of Pool.Call, mirroring Eval for a single
// Thread. It is a function rather than a method because Go does not allow
// methods to declare their own type parameters.
func EvalPool[T any](p *Pool, fn func() T) (T, bool) {
	var v T
	ok := p.Call(func() { v = fn() })
	return v, ok
}
