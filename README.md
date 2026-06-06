# thread

> [!WARNING]
> **This package has moved and this repository is archived.**
>
> `thread` now lives in the
> [`golang.design/x/runtime`](https://github.com/golang-design/runtime)
> module as [`golang.design/x/runtime/thread`](https://pkg.go.dev/golang.design/x/runtime/thread).
> Update your imports:
>
> ```diff
> -import "golang.design/x/thread"
> +import "golang.design/x/runtime/thread"
> ```
>
> The API is unchanged. This repository is no longer maintained; all
> future development happens in `golang.design/x/runtime`.

Package thread provides threading facilities, such as scheduling
calls on a specific thread, local storage, etc.

```go
import "golang.design/x/runtime/thread"
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