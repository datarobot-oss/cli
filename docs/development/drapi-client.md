# drapi client conventions

`internal/drapi` is the one shared HTTP client every authenticated call to
the DataRobot API is expected to build on. This page is the dev-doc mirror
of `internal/drapi/doc.go` (`go doc ./internal/drapi` for the canonical
source) — read this first when wiring in a new API call; read the package
doc when you need the full detail.

## Always build on the verb functions

Use `drapi.Get`, `drapi.Post`, `drapi.Patch`, or `drapi.Delete` (or their
`*JSON` variants) with a URL from `drapi.EndpointURL()` /
`config.GetEndpointURL()`. Never construct your own `http.Client` or set
the `Authorization` header by hand — `AuthorizeRequest` (`internal/drapi/auth.go`)
is called internally by every verb function and sets the bearer token,
`User-Agent`, and the optional API-consumer trace header consistently.

If you must build your own `*http.Request` (multipart uploads, custom error
decoding), use `drapi.Do(req, timeout)` rather than duplicating
`NewHTTPClient(t).Do(req)` — it applies the same timeout clamp described
below.

## Timeouts

Every verb function takes an optional trailing `time.Duration`. Omit it, or
pass `<=0`, and `NewHTTPClient` clamps it to `DefaultClientTimeout` (30s)
internally — the clamp cannot be bypassed by a caller. This was tightened in
CFX-7822 specifically so a stray `0` or negative duration could never
produce an unbounded request.

## The `HTTPError` / `errors.As` contract

A non-2xx response becomes a `*drapi.HTTPError{StatusCode, URL, Detail, Body}`,
unpackable via `errors.As`. Roughly 32 call sites across the codebase depend
on this contract holding — treat it as load-bearing when touching
`internal/drapi`.

`Body` is the raw, uninterpreted response body (different DataRobot APIs
disagree on their error envelope shape); `Detail` is best-effort and
populated by the caller — see `internal/workload/apiclient.LiftDetail` for
the pattern of parsing a FastAPI `{"detail": ...}` envelope into `Detail`.

**Known gap, don't copy this as the pattern**: `Get()` currently builds a
bare `&HTTPError{StatusCode, URL}` on failure instead of calling
`ErrFromResp()` the way `Post`, `Patch`, and `Delete` do — so a `GET`
failure carries no `Body`/`Detail` today, unlike every other verb. This is
a known asymmetry, not an intentional design choice; a fix is a reasonable
follow-up, but a new caller should not assume `Get()` failures carry the
same diagnostic detail as the others until it's closed.

## Origin safety

Before attaching the bearer token to any URL your code did not build
locally — a server-returned pagination cursor, a workload's live `endpoint`
URL — call `drapi.URLMatchesConfiguredBase(rawURL)` first and skip
authorization when it doesn't match. Attaching a CLI credential to an
arbitrary server-supplied URL would leak it to whatever origin that URL
points at.

Two worked examples:

- `AssertNextOnSameHost` (`internal/drapi/pagination.go`) guards a paginated
  list response's `Next` cursor before the client follows it.
- `endpointCheckAuthForURL` (`internal/workload/up/endpoint_check.go`)
  authenticates a post-deploy endpoint check only when the URL is the
  DataRobot-hosted workload gateway path *and* matches the configured base;
  a direct/customer-container URL always stays anonymous, so the CLI token
  never lands in a customer container's access log (RAPTOR-19741).

## See also

- [Authentication](authentication.md) — the OAuth-style login flow that
  produces the token these conventions consume.
- `go doc ./internal/drapi` — the full package doc.
