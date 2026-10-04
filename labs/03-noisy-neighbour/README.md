# Lab 03 — noisy neighbour

## Question

All tenants share one queue and one pool of worker loops, served oldest due
first. When one tenant sends a bulk of 500,000 deliveries, or one tenant's
endpoint answers slowly, how late do the other tenants' webhooks arrive?

## Prediction (written before the first run)

No prediction was written before run 1.

## Why this shape

Lab 02 showed that one healthy workload is easy. This lab adds a second
tenant that takes far more than its share of the worker, in two ways, one at
a time:

- **Bulk:** one tenant with 50 endpoints sends 10,000 messages in 20 s, as a
  bulk import would. Fan-out turns that into 500,000 deliveries. At the API it
  looks small (500 req/s); at the worker it is 25,000 deliveries/s.
- **Slow endpoint:** one tenant's endpoint takes 5 s to answer. Every send to
  it holds a worker loop for 5 s, so 10 msg/s needs 50 loops; the worker has 30.

Real case: Hookdeck describes a customer whose one Shopify store sent 30
million events in five minutes, and every other store's order, fulfillment,
and inventory webhooks waited for hours
([Hookdeck](https://hookdeck.com/blog/delivery-groups-early-access)).

Quiet tenants send for the whole run. Their lag is the measurement; the other
two groups are the disturbance.

```
0:00 ─ quiet: 9 tenants, 1,000 msg/s, 1 endpoint each ─────────────── 13:00
       1:00 bulk: 500 msg/s × 20 s × 50 endpoints
                                         10:00 slow: 10 msg/s, 5 s ── 12:00
```

| Group | Tenants × endpoints | API rate | Deliveries/s | When |
|---|---|---|---|---|
| quiet | 9 × 1 (`/ok/q<i>`) | 1,000 req/s | 1,000 | 0:00–13:00 |
| bulk | 1 × 50 (`/ok/bulk/<j>`) | 500 req/s | 25,000 | 1:00–1:20 |
| slow | 1 × 1 (`/slow/s`) | 10 req/s | 10, each held 5 s | 10:00–12:00 |

The API peaks at 1,500 req/s during the bulk, above lab 02's 1,000. If k6
reports dropped iterations, the run does not count; lower `QUIET_RATE` and
note the rate this laptop can take.

Worker capacity is not known exactly. Lab 02 measured the HTTP send at
10.3 ms; with the database work after it, one loop does roughly 65–90
sends/s, so 30 loops do about 2,000–2,600/s. The quiet tenants take 1,000 of
that, so the bulk drains with the rest: about 500,000 ÷ 1,000–1,600 ≈ 5–8
minutes. The run itself measures the real capacity: while the backlog is
large, sends/s is flat at the ceiling.

The phases are spaced so one disturbance has drained before the next starts;
otherwise the slow endpoint would hide what the bulk did.

## Setup

| | |
|---|---|
| Worker | one process, `WORKER_COUNT=30`, `WORKER_BATCH_SIZE=10`, `skiplocked`, pool 40. Unchanged: run 1 measures the plain oldest-first claim |
| API | pool 16 |
| Receiver | `-latency 10ms -slow-latency 5s`, no failures; paths tell the tenant groups apart |
| Database | `webhook_lab_bench`, never reset; new tenants per run |

## Run

```bash
export POSTGRES_URL='postgres://localhost:5432/webhook_lab_bench?sslmode=disable'

go run ./cmd/receiver -latency 10ms -slow-latency 5s
POSTGRES_URL="$POSTGRES_URL&pool_max_conns=16&application_name=webhook-api" go run ./cmd/api
POSTGRES_URL="$POSTGRES_URL&pool_max_conns=40&application_name=webhook-worker" \
  WORKER_COUNT=30 WORKER_BATCH_SIZE=10 WORKER_CLAIM_MODE=skiplocked go run ./cmd/worker

labs/03-noisy-neighbour/run.sh run1-fifo
```

`run.sh` creates the tenants, runs k6, waits for the drain, and writes
`results/<name>-{k6,report,samples}.txt`. The report has lag per group and
phase, the quiet tenants' lag in 30 s buckets, and the worker metrics from
Prometheus, including the peak sends/s.

## Results

| Run | Claim | Quiet lag p50 / p99, base | Quiet lag p99, bulk drain | Quiet lag p99, slow | Bulk done after | Peak sends/s |
|---|---|---|---|---|---|---|
| 1 | oldest first (FIFO) | 0.18 / 0.43 s | 229.5 s (max 231 s) | 125.2 s (max 131 s) | 251.5 s after the first bulk message | 2,555 |

Run 1, 2026-10-03 17:04:59–17:19:42 (`RUN=20261003-170458`), after a 30 s
warm-up. Raw output in `results/run1-fifo-*.txt`.

**Validity:** k6 dropped 2,366 quiet-tenant iterations (0.3% of 788,835),
all around 17:07:01, during the bulk: API latency rose and the quiet scenario
reached its 600-VU limit. By this README's rule the run is not clean. The
quiet tenants sent about 3% fewer messages in the 20 s bulk window than
planned; the lag numbers barely depend on that, and the drop is itself a
finding (the bulk hurt the API too).

- 1,278,835 deliveries, all succeeded, 0 failed, 0 retried, 0 duplicates.
  Receiver and database agree.
- Quiet tenants, base phase: lag p50 0.18 s, p99 0.43 s.
- Bulk: pending jumped to 471,251. Quiet messages sent at 1:00–1:30 waited
  up to 231 s. The last bulk delivery finished 251.5 s after the first bulk
  message, but the quiet tenants' lag only returned to ~0 at 9:30: by then
  the backlog in front of them was their own deliveries, queued during the
  bulk.
- Slow endpoint: 10 msg/s at 5 s each, about 1% of all traffic, raised the
  quiet tenants' lag from 0.35 s to 126 s p99 within two minutes, and it took
  until about 2 min after the load stopped to drain.
- Worker: peak 2,555 sends/s (30 s rate) — the measured capacity with 30
  loops. Claim p50/p99 2.2 / 24.6 ms with up to 471,000 due rows (lab 02:
  0.84 / 7.1 ms). Worker pool waits 1.
- API: p50 0.87 ms, p95 287 ms, p99 527 ms, max 1.21 s. API pool waits
  411,272, average pool wait 39.6 ms per request (lab 02 run 2: 0.95 ms).
  The bulk's 50-row transactions at 500 req/s slowed every tenant's
  `POST /messages`, not only the worker.
- Autovacuum on `deliveries` ran at 17:07, 17:12, and 17:18; dead tuples
  reached 1.19 M between runs. The due index grew from 5.9 MB to 20 MB and
  did not shrink when pending fell.
- Machine: load average 3–14, swap 2.0–2.2 GB, flat.

Rates per minute, from Prometheus:

| Minute | API req/s | Worker sends/s | |
|---|---|---|---|
| 0–1 | 1,000 | 993 | base: the worker keeps up |
| 1–2 | 1,168 | 2,190 | bulk (500 req/s for 20 s); the worker runs flat out |
| 2–10 | ~1,000 | 1,600–2,140 | draining the backlog at full speed |
| 10–12 | 1,010 | ~500 | slow endpoint: the worker drops to a quarter |
| 12–14 | 1,000 → 0 | ~490 | slow sends already queued still hold loops |
| 14–15 | 0 | 1,154 | slow deliveries gone; the rest drains |

A loop claims 10 deliveries and sends them one after another, so one slow
delivery in a batch also delays the 9 healthy ones behind it.

## Run 2 — fair claim and an endpoint cap

Run 1 showed two separate problems, and run 2 fixes both, each behind its own
flag, so either can be switched off later to see what it did alone:

| Problem in run 1 | Fix | Flag |
|---|---|---|
| The bulk took every batch: quiet tenants waited behind 500,000 deliveries | Claim from the tenants in turns: every tenant's oldest due delivery first, then every tenant's second | `WORKER_CLAIM_MODE=fair` |
| The slow endpoint held every loop: the worker fell from ~2,000 to ~500 sends/s | At most N loops of a process send to one endpoint at once; a delivery over the cap is handed back for 1 s without an attempt | `WORKER_ENDPOINT_CONCURRENCY=10` |

The fixes act in different phases, so one run shows both: the bulk phase
tests the fair claim, the slow phase tests the cap. They do interact in the
slow phase: the fair claim alone would put a slow delivery in almost every
batch, so every loop would stall; the cap keeps that to 10 loops.

Why 10: the slow endpoint needs 10 msg/s × 5 s = 50 loops to keep up. A cap
of 10 leaves 20 loops, about 1,700 sends/s, for everyone else, and makes the
slow tenant pay for its own endpoint: its 1,200 deliveries drain at
10 ÷ 5 s = 2/s, so about 10 minutes.

The fair claim needs the index from migration 00002,
`deliveries_due_by_tenant_idx (tenant_id, next_attempt_at) WHERE status = 'pending'`.
On a backlog of 50,000 bulk and 100 per quiet tenant (built in a rolled-back
transaction), a batch of 10 held 8 bulk and 2 quiet deliveries with the
oldest-first claim, and 1 bulk and 1 from each of the 9 quiet tenants with
the fair claim, which took 0.7 ms with 161 tenants.

What is left on purpose:

- A loop still sends its batch one delivery after another. The 10 loops that
  hold a slow send also hold up to 9 healthy deliveries for 5 s.
- The fair claim reads every tenant on every claim. That is cheap with 161
  tenants and would not be with 100,000.
- The API is not changed: the bulk's 50-row inserts still compete with the
  quiet tenants' requests (run 1's third finding).

### Prediction

No prediction was written before run 2.

### Run

As run 1, with the worker started this way:

```bash
POSTGRES_URL="$POSTGRES_URL&pool_max_conns=40&application_name=webhook-worker" \
  WORKER_COUNT=30 WORKER_BATCH_SIZE=10 WORKER_CLAIM_MODE=fair WORKER_ENDPOINT_CONCURRENCY=10 \
  go run ./cmd/worker

labs/03-noisy-neighbour/run.sh run2-fair-cap
```

### Results

| Run | Claim | Cap | Quiet lag p50 / p99, base | Quiet lag p99, bulk drain | Quiet lag p99, slow | Bulk done after | Peak sends/s |
|---|---|---|---|---|---|---|---|
| 1 | oldest first | none | 0.18 / 0.43 s | 229.5 s | 125.2 s | 251.5 s | 2,555 |
| 2 | fair | 10 | 0.17 / 0.44 s | 1.36 s (max 5.1 s) | 2.93 s (max 11.7 s) | 816.4 s after the first bulk message | 2,086 |

Run 2, 2026-10-04 20:29:15–20:49:38 (`RUN=20261004-202914`), after a 30 s
warm-up. Raw output in `results/run2-fair-cap-*.txt`.

**Validity:** k6 dropped 10,772 iterations (1.4%; run 1: 2,366), starting
during the bulk, when the quiet scenario hit its 600-VU limit again. The API
was not changed, and the machine was much busier than in run 1: load average
6–36 (run 1: 3–14), with `mediaanalysisd` at up to 90% CPU before the run.
By this README's rule the run is not clean. The bulk tenant sent 9,654 of
its 10,000 messages (482,700 deliveries).

What the fixes changed for the quiet tenants:

| Quiet tenants, lag p99 | Run 1 (FIFO, no cap) | Run 2 (fair, cap 10) |
|---|---|---|
| base | 0.43 s | 0.44 s |
| during the bulk drain | 229.5 s | 1.36 s |
| during the slow endpoint | 125.2 s | 2.93 s |
| worst single delivery | 231 s | 11.7 s |

At 20:32, with 393,426 bulk deliveries pending, the quiet tenants had 120;
at 20:40, one minute into the slow phase, they had 415 and the slow tenant
644 (run 1 piled up ~30,000 quiet deliveries in that minute).

Who paid instead:

| | Run 1 | Run 2 |
|---|---|---|
| bulk tenant, lag p99 | 218.6 s | 762.7 s |
| bulk done after first bulk message | 251.5 s | 816.4 s |
| slow tenant, lag p99 | 129.9 s | 559.0 s |
| slow tenant, last delivery after its first | ~4 min | 622.7 s |

- The worker still sent ~1,200 deliveries/s during the slow phase (run 1:
  ~500), and postponed 174,598 deliveries in total because the slow endpoint
  was at its cap. The slow tenant drained at about 2/s, as designed.
- The bulk drained more slowly: ~600–825/s above the quiet load, against
  ~1,050 in run 1, so it overlapped the slow phase by about 4 minutes. Claim
  p50/p99 5.8 / 88.4 ms (run 1: 2.2 / 24.6 ms), peak 2,086 sends/s (run 1:
  2,555). Part of that is the fair claim and the postponed deliveries being
  claimed again every second; part is the busier machine. One run cannot
  split the two.
- The quiet tenants' small tail (p99 1–5 s in a few 30 s buckets, max 11.7 s)
  is what was left on purpose: the 10 loops that hold a slow send also hold
  up to 9 healthy deliveries for 5 s.
- API: p50 1.72 ms, p95 338 ms, p99 817 ms, max 3.09 s; average pool wait
  48.9 ms per request (run 1: 39.6 ms). Unchanged by design — the bulk still
  slows every tenant's `POST /messages`.
- 1,253,476 deliveries, all succeeded, 0 failed, 0 retried, 0 duplicates.

## Why the numbers changed

_To write after the run._
