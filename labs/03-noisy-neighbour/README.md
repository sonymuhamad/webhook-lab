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

## Why the numbers changed

_To write after the run._
