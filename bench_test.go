package thread_test

import (
	"testing"

	"golang.design/x/thread"
)

func BenchmarkThread_Call(b *testing.B) {
	th := thread.New()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		th.Call(func() {})
	}
}
func BenchmarkThread_CallV(b *testing.B) {
	th := thread.New()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = th.CallV(func() interface{} {
			return true
		}).(bool)
	}
}

func BenchmarkThread_TLS(b *testing.B) {
	th := thread.New()
	th.Call(func() {
		th.SetTLS(1)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = th.CallV(func() interface{} {
			return th.GetTLS()
		})
	}
}
