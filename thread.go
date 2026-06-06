// Copyright 2020 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

// Package thread provides threading facilities, such as scheduling
// calls on a specific thread, local storage, etc.
//
// Deprecated: this package has moved to golang.design/x/runtime/thread.
// Update imports to "golang.design/x/runtime/thread"; this repository is
// no longer maintained and will be archived.
package thread // import "golang.design/x/thread"

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// Thread represents a thread instance.
type Thread interface {
	// ID returns the ID of the thread.
	ID() uint64

	// Call runs fn on the thread and blocks until fn returns.
	// If the thread has been terminated, fn is discarded and Call
	// returns immediately without running it.
	//
	// If Terminate may run concurrently with Call, Call can return
	// before fn finishes (the call is abandoned). In that case a value
	// that fn writes to a captured variable must not be read after Call
	// returns, as fn may still be writing it. Use Eval, which returns
	// fn's result safely, when you need a value back.
	Call(fn func())

	// Go schedules fn to run on the thread without waiting for it to
	// complete. If the thread has been terminated, fn is discarded.
	Go(fn func())

	// SetTLS stores a value in the thread's local storage. It must be
	// called from within Call, Go, or Eval so the access happens on the
	// thread itself. For instance:
	//
	//   th := thread.New()
	//   th.Call(func() {
	//      th.SetTLS("store in thread local storage")
	//   })
	SetTLS(x any)

	// GetTLS returns the value stored in the thread's local storage. It
	// must be called from within Call, Go, or Eval so the access happens
	// on the thread itself. For instance:
	//
	//   th := thread.New()
	//   th.Call(func() {
	//      tls := th.GetTLS()
	//      // ... do whatever you want to do with the tls value ...
	//   })
	GetTLS() any

	// Terminate terminates the thread gracefully.
	// Scheduled but unexecuted calls are discarded.
	// It is safe to call Terminate multiple times, including
	// concurrently from multiple goroutines.
	Terminate()
}

// Eval runs fn on the thread and returns its result. It blocks until
// fn returns. If the thread has been terminated, fn is discarded and
// Eval returns the zero value of T.
//
// Eval is the typed counterpart of Call: it preserves fn's return type
// without an interface conversion or a type assertion at the call site.
//
//	th := thread.New()
//	n := thread.Eval(th, func() int { return 1 })
//
// Eval is a function rather than a method of Thread because Go does not
// allow methods to declare their own type parameters.
func Eval[T any](th Thread, fn func() T) T {
	// Hand the result back over a channel rather than a captured
	// variable. If Terminate races with this call, Call can return
	// while the worker is still inside fn; a captured variable would
	// then be read here while the worker writes it (a data race),
	// whereas a channel send/receive is synchronized. The channel is
	// buffered so the worker never blocks delivering the result.
	ch := make(chan T, 1)
	th.Call(func() { ch <- fn() })
	select {
	case v := <-ch:
		// Normal path: the worker sends on ch before signalling Call's
		// completion, so the value is already available here.
		return v
	default:
		// Terminate path: fn was discarded; return the zero value.
		var zero T
		return zero
	}
}

// New creates a new thread instance.
func New() Thread {
	th := &thread{
		id:   globalID.Add(1),
		fdCh: make(chan funcData, runtime.GOMAXPROCS(0)),
		quit: make(chan struct{}),
		once: &sync.Once{},
	}

	// The worker goroutine captures only the channels, never th itself.
	// If it captured th, the running goroutine would keep th reachable
	// forever, the cleanup below could never fire, and the goroutine
	// (with its locked OS thread) would leak whenever a caller drops a
	// Thread without calling Terminate.
	go worker(th.fdCh, th.quit)

	// As a safety net, stop the worker once th becomes unreachable.
	// runtime.AddCleanup forbids the cleanup func from referencing th,
	// which is exactly the property we need here: the closure touches
	// only the once and the quit channel.
	sd := shutdown{once: th.once, quit: th.quit}
	runtime.AddCleanup(th, func(s shutdown) {
		s.once.Do(func() { close(s.quit) })
	}, sd)

	return th
}

// shutdown carries the data needed to stop a thread's worker without
// referencing the Thread itself (a requirement of runtime.AddCleanup).
type shutdown struct {
	once *sync.Once
	quit chan struct{}
}

// worker runs on a dedicated, OS-locked thread and serially executes
// scheduled calls until quit is closed. It deliberately does not close
// over the owning thread value, so the thread can be garbage collected.
func worker(fdCh chan funcData, quit chan struct{}) {
	runtime.LockOSThread()
	// Note: the goroutine returns without UnlockOSThread, which makes the
	// runtime terminate the underlying OS thread. That is the intended
	// behavior for a dedicated thread.
	for {
		// Give quit priority: once terminated, scheduled but unexecuted
		// calls must be discarded, not run. Without this pre-check the
		// select below could pick a queued call over a ready quit.
		select {
		case <-quit:
			return
		default:
		}
		select {
		case <-quit:
			return
		case fd := <-fdCh:
			fd.run()
		}
	}
}

var (
	// donePool recycles the completion channels used by Call. The
	// channels are buffered (cap 1) so the worker's completion send
	// never blocks, even if the caller stopped waiting because the
	// thread was terminated. A channel is returned to the pool only once
	// it is known to be drained; a channel abandoned on the terminate
	// path is dropped (left to the GC) so a future call never receives a
	// dirty channel.
	donePool = sync.Pool{
		New: func() any {
			return make(chan struct{}, 1)
		},
	}
	globalID atomic.Uint64
	_        Thread = &thread{once: &sync.Once{}}
)

type funcData struct {
	fn   func()
	done chan struct{}
}

// run executes the scheduled call and reports completion on done, if
// any. done is buffered, so the send never blocks.
func (fd funcData) run() {
	if fd.done != nil {
		defer func() { fd.done <- struct{}{} }()
	}
	fd.fn()
}

type thread struct {
	id  uint64
	tls any

	fdCh chan funcData
	quit chan struct{}
	once *sync.Once
}

func (th *thread) ID() uint64 {
	return th.id
}

func (th *thread) Call(fn func()) {
	if fn == nil {
		return
	}

	// Don't enqueue after termination. A bare send/quit select would
	// race (both cases ready once quit is closed and the buffer has
	// room); this leading check makes a completed Terminate win
	// deterministically.
	select {
	case <-th.quit:
		return
	default:
	}

	done := donePool.Get().(chan struct{})
	select {
	case <-th.quit:
		donePool.Put(done) // unused, still clean
		return
	case th.fdCh <- funcData{fn: fn, done: done}:
	}

	select {
	case <-done:
		donePool.Put(done) // drained, safe to reuse
	case <-th.quit:
		// Terminated while waiting. The worker may still write to
		// done's buffer, so do not reuse it; let the GC reclaim it.
	}
}

func (th *thread) Go(fn func()) {
	if fn == nil {
		return
	}
	// See Call: don't enqueue after termination.
	select {
	case <-th.quit:
		return
	default:
	}
	select {
	case <-th.quit:
	case th.fdCh <- funcData{fn: fn}:
	}
}

func (th *thread) GetTLS() any {
	return th.tls
}

func (th *thread) SetTLS(x any) {
	th.tls = x
}

func (th *thread) Terminate() {
	th.once.Do(func() { close(th.quit) })
}
