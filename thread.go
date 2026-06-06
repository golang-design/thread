// Copyright 2020 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

// Package thread provides threading facilities, such as scheduling
// calls on a specific thread, local storage, etc.
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

	// Call calls fn from the given thread. It blocks until fn returns.
	Call(fn func())

	// CallNonBlock call fn from the given thread without waiting
	// fn to complete.
	CallNonBlock(fn func())

	// CallV call fn from the given thread and returns the returned
	// value from fn.
	//
	// The purpose of this function is to avoid value escaping.
	// In particular:
	//
	//   th := thread.New()
	//   var ret any
	//   th.Call(func() {
	//      ret = 1
	//   })
	//
	// will cause variable ret be allocated on the heap, whereas
	//
	//   th := thread.New()
	//   ret := th.CallV(func() any {
	//     return 1
	//   }).(int)
	//
	// will offer zero allocation benefits.
	CallV(fn func() any) any

	// SetTLS stores a given value to the local storage of the given
	// thread. This method must be accessed in Call, or CallV, or
	// CallNonBlock. For instance:
	//
	//   th := thread.New()
	//   th.Call(func() {
	//      th.SetTLS("store in thread local storage")
	//   })
	SetTLS(x any)

	// GetTLS returns the locally stored value from local storage of
	// the given thread. This method must be access in Call, or CallV,
	// or CallNonBlock. For instance:
	//
	//   th := thread.New()
	//   th.Call(func() {
	//      tls := th.GetTLS()
	//      // ... do what ever you want to do with tls value ...
	//   })
	//
	GetTLS() any

	// Terminate terminates the given thread gracefully.
	// Scheduled but unexecuted calls will be discarded.
	// It is safe to call Terminate multiple times, including
	// concurrently from multiple goroutines.
	Terminate()
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
		select {
		case <-quit:
			return
		case fd := <-fdCh:
			fd.run()
		}
	}
}

var (
	// donePool and varPool recycle the result channels used by Call and
	// CallV. The channels are buffered (cap 1) so the worker's result
	// send never blocks, even if the caller stopped waiting because the
	// thread was terminated. A channel is returned to the pool only once
	// it is known to be drained; a channel abandoned on the terminate
	// path is dropped (left to the GC) so a future call never receives a
	// dirty channel.
	donePool = sync.Pool{
		New: func() any {
			return make(chan struct{}, 1)
		},
	}
	varPool = sync.Pool{
		New: func() any {
			return make(chan any, 1)
		},
	}
	globalID atomic.Uint64
	_        Thread = &thread{once: &sync.Once{}}
)

type funcData struct {
	fn   func()
	done chan struct{}

	fnv func() any
	ret chan any
}

// run executes the scheduled call and reports completion on the
// corresponding result channel, if any. The result channels are
// buffered, so these sends never block.
func (fd funcData) run() {
	switch {
	case fd.fn != nil:
		if fd.done != nil {
			defer func() { fd.done <- struct{}{} }()
		}
		fd.fn()
	case fd.fnv != nil:
		var ret any
		if fd.ret != nil {
			defer func() { fd.ret <- ret }()
		}
		ret = fd.fnv()
	}
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

func (th *thread) CallNonBlock(fn func()) {
	if fn == nil {
		return
	}
	select {
	case <-th.quit:
	case th.fdCh <- funcData{fn: fn}:
	}
}

func (th *thread) CallV(fn func() any) (ret any) {
	if fn == nil {
		return nil
	}

	out := varPool.Get().(chan any)
	select {
	case <-th.quit:
		varPool.Put(out) // unused, still clean
		return nil
	case th.fdCh <- funcData{fnv: fn, ret: out}:
	}

	select {
	case ret = <-out:
		varPool.Put(out) // drained, safe to reuse
		return ret
	case <-th.quit:
		// Terminated while waiting; abandon out (see Call).
		return nil
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
