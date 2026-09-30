# Lab 02 — large table

## Question

The bench database holds 20 million finished deliveries from 100 other
tenants. At 1,000 messages per second for 5 minutes, with 5% of sends
answered with 500, how do `POST /messages`, the worker's claim, and the
retries behave?

## Prediction (written before the first run)

Written by Sony on 2026-09-30, before the run.

| # | Prediction | Number | Why |
|---|---|---|---|
| 1 | API p50 / p99 | p50 500 ms – 1 s, p99 1 – 2 s | At this scale it is not much for Postgres; 5 million rows is nothing for Postgres writes. (The table holds 20 M, not 5 M.) |
| 2 | `dropped_iterations` | 0 | The laptop can take 1,000 req/s after freeing up resources for the run. |
| 3 | Claim p50 / p99 | about 50–100 ms worse per claim | No problem: the index is there and the query is optimized. |
| 4 | Sends/s, pending due peak | pending peaks, then goes down after several minutes | — |
| 5 | Retried / failed deliveries, duplicates | more retries than lab 01, 0 failed, 0 duplicates | The chaos adds retries; the outbox retries them all. |
| 6 | Drain time | 1 – 5 min | — |
| 7 | Delivery lag p50 / p99 | lower than lab 01 | Lag comes from receiver latency and the claim query. Receiver latency is lower, and 20 M rows are no problem for an optimized query on an indexed table. |
| 8 | Autovacuum | no prediction | Claude tracks `n_dead_tup` and `last_autovacuum` before and after, and explains the trigger afterwards. A dedicated autovacuum lab comes later. |
| 9 | First bottleneck | the receiver | — |

## Why this shape

Lab 01 ran on an empty table at 200 messages/s. Its `SKIP LOCKED` run is the
empty-table baseline here: the bench database keeps the data of every lab, so
an empty run is not possible any more.

| | Lab 01 (baseline) | Lab 02 |
|---|---|---|
| History | none | 100 tenants × 50 endpoints × 4,000 messages = 20 M succeeded deliveries, 30 days |
| Load | 200 msg/s × 60 s, 1 tenant | 1,000 msg/s × 5 min, 10 live tenants with 1 endpoint each |
| Receiver | 50 ms | 10 ms, 5% answered with 500 |
| Worker | 3 loops | 30 loops, batch 10, pool of 40 connections |

The two runs differ in more than the history, so compare them with care: a
difference in API latency can come from the higher rate, not from the table.

What the history should and should not affect:

- The claim reads `deliveries_due_idx`, a partial index over pending rows
  only. The seeded rows are finished, so they are not in it: the index was
  8 KB after seeding.
- Every insert still updates the full indexes: the primary keys and
  `UNIQUE (message_id, endpoint_id)` on `deliveries` (951 MB after seeding),
  and the idempotency index on `messages`.

The live tenants have one endpoint each, so 1,000 msg/s is 1,000
deliveries/s. With 50 endpoints each, it would be 50,000 deliveries/s, and
the worker could not keep up on one laptop.

Why 30 loops: one loop sends one delivery at a time, about 15 ms each with a
10 ms receiver, so about 65 sends/s per loop. 1,000 deliveries/s plus about
50 retries/s needs about 16 loops; 30 leaves headroom. The batch size does
not add throughput, because a loop sends its batch one delivery after
another.

What the 5% failures should do: each 500 leaves the delivery pending, with
`next_attempt_at` 30 s later (`DELIVERY_RETRY_DELAY`), up to 5 attempts. The
chance of 5 failures in a row is 0.05⁵, about 1 in 3.2 million, so the run
should end with 0 `failed` deliveries, about 15,000 retried deliveries
(300,000 × 0.05), about 15,800 failed attempts (15,000 + 750 + 38 + 2), and
0 duplicates. The last retries arrive at least 30 s after k6 stops, so the
drain time includes one retry delay.

## Setup

| | |
|---|---|
| Load | k6 `constant-arrival-rate`, 1,000 req/s for 5 min → 300,000 messages |
| Tenants | 10 live tenants `lab-02-<run>-*`, one endpoint each; each request picks one at random |
| Worker | one process, `WORKER_COUNT=30`, `WORKER_BATCH_SIZE=10`, `WORKER_CLAIM_MODE=skiplocked`, pool of 40 connections |
| Receiver | `cmd/receiver -latency 10ms -fail-rate 0.05` |
| Database | `webhook_lab_bench`, never reset; history from `seed.sql` |
| Machine | one laptop runs everything: compare runs, do not trust absolute numbers |

The history was seeded once, on 2026-09-29, in 12 min 22 s:

| Table | Rows | Size |
|---|---|---|
| messages | 400,000 | 57 MB |
| deliveries | 20,000,000 | 4,115 MB |
| attempts | 20,000,000 | 2,985 MB |
| database | | 7,166 MB |

## Run

Each process in its own terminal, from the repo root. Prometheus, Loki, and
Grafana must run: `make obs-status`.

```bash
export POSTGRES_URL='postgres://localhost:5432/webhook_lab_bench?sslmode=disable'

go run ./cmd/receiver -latency 10ms -fail-rate 0.05              # terminal 1, restart before every run
go run ./cmd/api                                                 # terminal 2
POSTGRES_URL="$POSTGRES_URL&pool_max_conns=40" \
  WORKER_COUNT=30 WORKER_BATCH_SIZE=10 WORKER_CLAIM_MODE=skiplocked \
  go run ./cmd/worker                                            # terminal 3

# terminal 4
labs/02-large-table/setup.sh                                     # new live tenants, no seeding
source labs/02-large-table/.lab.env
start=$(date +%s)
k6 run -e API_KEYS="$API_KEYS" -e RATE=1000 -e VUS=1000 -e DURATION=5m \
  labs/01-worker-contention/load.js
RUN=$RUN labs/02-large-table/report.sh                           # waits for the queue to drain
labs/02-large-table/metrics.sh "$start" <drained at>
```

`report.sh` prints `drained at <unix time>`; pass that to `metrics.sh`.

To add more history later: `HISTORY_MESSAGES=4000 labs/02-large-table/setup.sh`
adds another 100 tenants × 50 endpoints × 4,000 messages.

A run counts only when k6 reports `dropped_iterations` 0 and no failed
requests. If run 1 drops iterations, lower `RATE` and write down the rate
that one laptop can take.

## Results

| Run | History | API p50 / p99 | Claim p50 / p99 | Sends/s | Failed sends | Retried / failed deliveries | Duplicates | Drain time | Delivery lag p50 / p99 | API pool waits |
|---|---|---|---|---|---|---|---|---|---|---|
| Lab 01 `SKIP LOCKED` | none | – / 12.5 ms | | ~54 | 0 | 0 / 0 | 0 | 218 s | 79 s / 292 s | 7 |
| 1 | 20 M | 0.65 ms / 21.5 ms | 0.84 ms / 7.1 ms | ~1,050 during load (789 avg over 405 s) | 15,816 | 15,074 / 0 | 0 | 104 s | 0.20 s / 55.2 s | 10,226 |

Run 1, 2026-09-30 17:59–18:06 (`RUN=20260930-175926`), after a 30 s warm-up at
100 req/s on separate tenants. Raw output in `results/run1-*.txt`.

- k6: 300,001 requests at 1,000.0/s, 0 dropped iterations, 0 failed. Max 508 ms.
- Receiver and database agree: 300,001 unique deliveries, 15,816 answered with
  500, 0 duplicates. (The receiver was not restarted after the warm-up, so its
  raw totals include the warm-up's 3,001 and 166.)
- Pending stayed between 1,550 and 2,000 during the load, then fell to 4
  within a minute. The due peak was 295; the rest waited for their retry delay.
- Autovacuum ran on `deliveries` every minute (count 28 → 33), and dead tuples
  stayed flat at about 54,000. See below for why.
- Worker pool waits 0; API pool waits 10,226 (pool of 8, the default).
- Machine: load average 14–25 during the run (Spotlight re-indexing after a
  boot), swap 0 MB throughout.

Why autovacuum ran at all: the seed's `VACUUM` left 99.7% of `deliveries`
pages all-frozen. Since Postgres 18 the insert trigger scales with the
unfrozen part of the table only: 1,000 + 0.2 × 20 M × 0.003 ≈ 13,000 inserts,
not 4 M. The run inserted 60,000 rows a minute, so autovacuum ran on every
1-minute naptime.

## Why the numbers changed

_To write after the run._
