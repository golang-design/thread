# thread [![PkgGoDev](https://pkg.go.dev/badge/golang.design/x/thread)](https://pkg.go.dev/golang.design/x/thread) [![Go Report Card](https://goreportcard.com/badge/golang.design/x/thread)](https://goreportcard.com/report/golang.design/x/thread) ![thread](https://github.com/golang-design/thread/workflows/thread/badge.svg?branch=main)

Package thread provides threading facilities, such as scheduling
calls on a specific thread, local storage, etc.

```go
import "golang.design/x/thread"
```

## Quick Start

```go
th := thread.New()
defer th.Terminate()

// Run on the thread and block until it returns.
th.Call(func() {
    // ... runs on the created thread ...
})

// Schedule on the thread without waiting.
th.Go(func() {
    // ... runs on the created thread ...
})

// Run on the thread and return a typed value.
n := thread.Eval(th, func() int {
    return 42
})
```

## License

MIT &copy; 2020 - 2026 The golang.design Initiative