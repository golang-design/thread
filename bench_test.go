package thread_test

import (
	"testing"

	"golang.design/x/thread"
)

func BenchmarkThread_Call(b *testing.B) {
	th := thread.New()
	defer th.Terminate()
	b.ReportAllocs()

	for b.Loop() {
		th.Call(func() {})
	}
}

func BenchmarkThread_Go(b *testing.B) {
	th := thread.New()
	defer th.Terminate()
	b.ReportAllocs()

	for b.Loop() {
		th.Go(func() {})
	}
}

func BenchmarkThread_Eval(b *testing.B) {
	th := thread.New()
	defer th.Terminate()
	b.ReportAllocs()

	for b.Loop() {
		_ = thread.Eval(th, func() bool {
			return true
		})
	}
}

func BenchmarkThread_TLS(b *testing.B) {
	th := thread.New()
	th.Call(func() {
		th.SetTLS(1)
	})
	defer th.Terminate()

	b.ReportAllocs()
	for b.Loop() {
		_ = thread.Eval(th, func() any {
			return th.GetTLS()
		})
	}
}
