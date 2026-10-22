# HTTP cancellation and request timeouts

The `native:net/http/v1` module supports an optional per-request timeout:

```lisp
(import
  (http @native:net/http/v1)
  (http/request "GET" "https://example.com/" null (bytes/concat)
    (http/timeout 5000)))
```

The timeout is a positive integer number of milliseconds covering the complete
request, including response-body reading. Omitting the option uses 10,000 ms.
An explicit timeout replaces that default and may be shorter or longer. Zero,
negative values, malformed options and durations that overflow are rejected.

Every request derives from the current evaluation context. An earlier enclosing
deadline still applies, and evaluation cancellation interrupts the request at
any point. The HTTP client does not impose a second fixed total-request timeout.
Underlying transport-specific connection limits still apply.

A request-local timeout is a catchable error (`http/request timed out`). Go
callers can identify its cause with `errors.Is(err, context.DeadlineExceeded)`.
The enclosing evaluation can continue after catching that error. Cancellation
or expiry of the evaluation itself instead propagates `lisp.ErrInterrupted`
with its context cause, including through `error/catch`.

HTTP status codes, including 408 and 504, remain ordinary response values when
their bodies have been read successfully. Failed body reads return errors rather
than partial response values. Response bodies are closed on exit. The existing
10 MiB response-body bound, redirect behavior and taint propagation still apply.

`web/search` and `web/fetch` also use the current evaluation context, including
DNS validation for fetches. They retain their respective 25-second and 30-second
request defaults and their existing body limits. Their request timeouts are
catchable; enclosing evaluation interruption propagates normally.

Go host functions created with `lisp.FunctionWithContext` or its non-leaky
variant can access the same context via `call.Context()` when performing I/O.
