// Copyright 2020 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

// +build linux

package thread_test

import (
	"strings"
	"sync"
	"testing"

	"golang.design/x/thread"
	"golang.org/x/sys/unix"
)

func TestNew(t *testing.T) {
	th1 := thread.New()
	th2 := thread.New()

	if th1.ID() == th2.ID() {
		t.Fatalf("two different threads have same id")
	}
}

func TestThread_Call(t *testing.T) {
	th := thread.New()
	defer th.Terminate()
	osThreadID := 0

	th.Call(func() {
		osThreadID = unix.Gettid()
		t.Logf("thread id: %v", osThreadID)
	})

	var shouldFail bool
	th.Call(func() {
		if unix.Gettid() != osThreadID {
			shouldFail = true
		}
		osThreadID = unix.Gettid()
		t.Logf("thread id: %v", osThreadID)
	})
	if shouldFail {
		t.Fatalf("failed to schedule function call on the same thread.")
	}
}

func TestThread_Terminate(t *testing.T) {
	th := thread.New()

	th.Terminate()
	th.Terminate()
	th.Terminate() // thread should be able to terminate multiple times

	th.Call(func() {
		panic("call should not be scheduled on a terminated thread.")
	})
	th.Call(func() {
		panic("call should not be scheduled on a terminated thread.")
	})
}

func TestThread_CallNonBlock(t *testing.T) {
	th := thread.New()
	defer th.Terminate()

	th.CallNonBlock(func() {
		th.SetTLS(1)
	})

	v := th.CallV(func() interface{} {
		return th.GetTLS()
	}).(int)
	if v != 1 {
		t.Fatalf("non blocking call is not scheduled before a blocking call.")
	}
}

func TestThread_CallV(t *testing.T) {
	th := thread.New()
	defer th.Terminate()

	osThreadID1 := th.CallV(func() interface{} {
		return unix.Gettid()
	}).(int)

	osThreadID2 := th.CallV(func() interface{} {
		return unix.Gettid()
	}).(int)
	if osThreadID1 != osThreadID2 {
		t.Fatalf("failed to schedule function call on the same thread.")
	}
}

func TestThread_TLS(t *testing.T) {
	th1 := thread.New()
	th1.SetTLS("hello")

	th2 := thread.New()
	th2.SetTLS("world")

	tls1 := th1.CallV(func() interface{} {
		return th1.GetTLS()
	}).(string)
	if strings.Compare(tls1, "hello") != 0 {
		t.Fatalf("incorrect TLS access")
	}
	t.Log(tls1)
	tls2 := th2.CallV(func() interface{} {
		return th2.GetTLS()
	}).(string)
	if strings.Compare(tls2, "world") != 0 {
		t.Fatalf("incorrect TLS access")
	}
	t.Log(tls2)
}

func TestThread_TLSConcurrent(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	th := thread.New()
	go func() {
		defer wg.Done()
		fn := func() {
			th.SetTLS(1)
		}
		th.Call(fn)
	}()
	go func() {
		defer wg.Done()
		fn := func() {
			th.SetTLS(2)
		}
		th.Call(fn)
	}()
	wg.Wait()
}
