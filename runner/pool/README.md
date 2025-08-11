# Pool Runner – Technical Documentation for `pool.go`

> **Scope:** This document describes the behavior, API, data structures, lifecycle, and operational caveats of the **current** single-file implementation found in `runner/pool/pool.go`.

---

## 1) Overview

The pool runner maintains **idle** (pre-warmed) Docker containers so invocations can reuse a running container (warm start) and avoid part of the cold‑start latency. When an idle container is unavailable, the pool **cold‑starts** a new container using:

```
docker run -d --rm <image> tail -f /dev/null
```

Execution inside a pooled container is performed via:

```
docker exec <container-id> <args...>
```

The pool uses a **time-to-live (TTL)** to limit the lifetime of containers and a **janitor** goroutine to evict expired idle containers periodically.

---

## 2) Public API

### `Manager` interface

```go
Acquire(ctx context.Context, image string) (Container, error)
Release(ctx context.Context, c Container) error
Shutdown(ctx context.Context) error
```

* **Acquire** (non-blocking): returns an *idle* container for `image` if available; otherwise cold‑starts a new one. (No waiting semantics.)
* **Release**: attempts to return a container to the idle pool; evicts it if expired by TTL.
* **Shutdown**: stops the janitor and closes all *idle* containers.

### `Container` interface

```go
ID() string
Exec(ctx context.Context, args []string) ([]byte, error)
Close(ctx context.Context) error
```

* **Exec** returns **combined stdout+stderr** (`CombinedOutput`).
* **Close** force-removes the container (`docker rm -f`).

---

## 3) Internal Types & Data Structures

```go
type pooledContainer struct {
  id        string       // Docker container ID
  createdAt time.Time    // spawn time
  lastUsed  time.Time    // updated on Acquire
}

type warmPoolManager struct {
  ttl             time.Duration       // eviction age threshold
  maxSize         int                 // soft cap on idle list length (per key)
  cleanupInterval time.Duration       // janitor tick (defaults to 1m)
  mu              sync.Mutex          // guards the pool map
  pool            map[string][]*pooledContainer
  stopJanitor     chan struct{}
}
```

> **Note:** The map comment says *“image → idle list”*. See [Section 8](#8) for current implementation inconsistencies
---

## 4) Lifecycle & Control Flow

### Acquire

1. Lock pool.
2. If there’s an idle container slice under `pool[image]` and it’s non-empty, take the **first** entry, set `lastUsed = now`, return it.
3. If the idle list length `len(pool[image]) ≥ maxSize`, evict the **oldest** idle (index 0) via `Close`.
4. Cold‑start a new container (still while holding the mutex) via `docker run -d --rm ...`.
5. Return the new container.

**Notes:**

* No TTL check is performed at reuse time (expired idles can be handed out; TTL is enforced during `Release` and by the janitor).
* The mutex is held across the cold‑start operation, potentially blocking other acquisitions/releases while Docker starts the container.

### Release

1. If the container is not a `*pooledContainer`, close it.
2. If `now - createdAt > ttl`, close it.
3. Else, append it back to the idle list **(see §8: key choice)**.

### Janitor

* Ticks every `cleanupInterval` (default: **1 minute**).
* Under lock, iterates all idle lists and closes any container whose age exceeds `ttl`.
* Calls to `Close` happen while the lock is held.

### Shutdown

* Signals janitor to stop.
* Under lock, closes all containers present in idle lists and clears the map.

---

## 5) Concurrency & Synchronization Model

* A single `sync.Mutex` protects the `pool` map and list mutations.
* **Acquire** and **janitor eviction** both perform work *while holding the mutex* (including potentially long‑running Docker CLI calls). This can serialize operations and increase contention under load.
* There is **no blocking** or condition variable: the pool never waits for a container to be released; it always reuses or cold‑starts.

---

## 6) Semantics: TTL & Capacity

* **TTL**: Enforced on `Release` and by the janitor. **Acquire** does not filter expired idles before reuse.
* **Capacity (`maxSize`)**: Applies to the **length of the idle list** only (soft cap). It does **not** limit concurrent active containers. When the idle list reaches `maxSize`, `Acquire` evicts the oldest idle before cold‑starting a new container.

---

## 7) Execution & Error Handling

* **Cold start** uses `docker run -d --rm <image> tail -f /dev/null` and captures the container ID from `stdout` **without trimming** newline.
* **Exec** uses `docker exec` with **combined** output. On non‑zero exit, the output is still returned alongside a wrapped error.
* **Close** uses `docker rm -f` to force remove; errors are wrapped and surfaced.

---

## 8) Known Inconsistencies & Behavioral Quirks (as-is)

1. **Keying on `Release`**
   `Release` appends the container back using `m.pool[pc.id] = append(...)`. The map’s intended key is **image**, but `pc` does not store the image, and the code uses **container ID** as the key. This causes:

   * Idle containers to be placed under a key that `Acquire(image)` never looks up.
   * Effective loss of reuse for released containers (they won’t be found on next `Acquire(image)`).

2. **TTL not checked on Acquire**
   Expired containers can be handed out and only evicted when later released or by the janitor.

3. **Lock held during external calls**
   `Acquire` (cold start) and janitor eviction call Docker while the mutex is held, potentially blocking other threads.

4. **Container ID formatting**
   The returned ID from `docker run` is not `TrimSpace`d; it may include a trailing newline.

5. **`lastUsed` timing**
   Updated on `Acquire`, not on `Release`. (No logic depends on `lastUsed` currently.)

6. **Eviction order**
   The idle list always removes/evicts from index `0` (“oldest” as maintained) and reuses also from index `0` (not LIFO). This is acceptable but should be documented explicitly.

---

## 9) Usage Example

TBA
---

## 10) Operational Notes

* Ensure the Docker daemon is reachable and the image is present/pullable.
* Pool is **in‑memory only**; on process restart, warm containers not tracked as idle will not be managed/shutdown by this code.
* Because the pool never blocks, bursts of traffic may create many active containers even when `maxSize` is small.

---

## 11) Security/Permissions

* The process must have permission to run Docker CLI commands (`docker run`, `docker exec`, `docker rm`).
* Consider isolating images/containers appropriately and handling sensitive arguments passed to `Exec`.

---

## 12) Future Enhancements (non-breaking)

* **Fix keying on `Release`** (store `image` on `pooledContainer` and key by image consistently).
* **Trim container ID** from `docker run` output.
* **TTL check on Acquire** to avoid handing out expired idles.
* **Reduce lock hold time** (perform Docker operations outside the critical section).
* **Optional LIFO reuse** (take newest idle) for better cache locality.
* **Configurable janitor interval**.
* **Metrics hooks**: hits/misses, acquire latency, evictions.

---

*End of document.*
