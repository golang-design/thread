// Copyright 2020 The golang.design Initiative Authors.
// All rights reserved. Use of this source code is governed
// by a MIT license that can be found in the LICENSE file.

package thread_test

import (
	"testing"

	"golang.design/x/thread"
)

// TestThread_IDConcurrentTLS guards against ID() using a value receiver:
// a value receiver copies the whole thread struct (including the tls
// field) on every call, which races with SetTLS. Run with -race.
func TestThread_IDConcurrentTLS(t *testing.T) {
	th := thread.New()
	defer th.Terminate()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 1000 {
			th.Call(func() { th.SetTLS(i) })
		}
	}()

	for range 1000 {
		_ = th.ID()
	}
	<-done
}
