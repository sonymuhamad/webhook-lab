# Lab 02 — large table

## Question

The `deliveries` table holds 5 million finished deliveries. At 1,000 messages
per second, how much slower are `POST /messages` and the worker's claim than
on an empty table?

## Prediction (written before the first run)

_To write before run A._

## Why this shape

Lab 01 started every run from an empty table, at 200 messages/s. This lab
raises the rate to 1,000/s and changes one variable between the two runs: the
amount of history already in the tables.

| Run | History | Load |
|---|---|---|
| A (baseline) | none | 1,000 req/s × 60 s |
| B | 5,000,000 succeeded deliveries, with their messages and attempts, spread over 30 days | 1,000 req/s × 60 s |

Run A also answers whether one laptop can take 1,000 req/s at all. If k6
reports dropped iterations or failed requests in run A, lower `RATE` and use
that rate for both runs.

What the history should and should not affect:

- The claim reads `deliveries_due_idx`, a partial index over pending rows
  only. Finished rows are not in it, so the claim should not slow down.
- Every insert still updates the full indexes: the primary keys and
  `UNIQUE (message_id, endpoint_id)` on `deliveries`, and the idempotency
  index on `messages`. Those indexes are 5 million entries deep in run B.

## Setup

| | |
|---|---|
| Load | k6 `constant-arrival-rate`, 1,000 req/s for 60 s → 60,000 messages |
| Tenant | `lab-02`, one endpoint |
| Worker | one process, `WORKER_COUNT=60`, `WORKER_CLAIM_MODE=skiplocked`, pool of 40 connections |
| Receiver | `cmd/receiver -latency 50ms` |
| Database | `webhook_lab_bench`, reset before each run; `seed.sql` fills history for run B |
| Machine | one laptop runs everything: compare the two runs, do not trust absolute numbers |

Why 60 loops: one loop sends one delivery at a time, about 55 ms each, so
keeping up with 1,000/s takes about 1,000 × 0.055 ≈ 55 loops. With fewer, the
backlog grows during the run and delivery lag measures the backlog, not the
table size. The worker's pool is raised to 40 through `pool_max_conns` in the
connection URL; with the default of 8, the loops would queue for connections
instead.

## Run

Each process in its own terminal, from the repo root.

```bash
export POSTGRES_URL='postgres://localhost:5432/webhook_lab_bench?sslmode=disable'

go run ./cmd/receiver -latency 50ms                              # terminal 1, restart before every run
go run ./cmd/api                                                 # terminal 2
POSTGRES_URL="$POSTGRES_URL&pool_max_conns=40" \
  WORKER_COUNT=60 WORKER_CLAIM_MODE=skiplocked go run ./cmd/worker  # terminal 3

# terminal 4 — run A
labs/02-large-table/setup.sh
source labs/02-large-table/.lab.env
date +%s                                                         # note the start time
k6 run -e API_KEY="$API_KEY" -e RATE=1000 -e VUS=1000 labs/01-worker-contention/load.js
labs/01-worker-contention/report.sh                              # waits for the queue to drain
date +%s                                                         # note the end time
labs/02-large-table/metrics.sh <start> <end>

# run B: restart the receiver, then the same with history
ROWS=5000000 labs/02-large-table/setup.sh                        # seeding takes several minutes
```

`load.js` is lab 01's script; `RATE` and `VUS` override its defaults.
`report.sh` from lab 01 works unchanged against the same bench database.

A run counts only when k6 reports `dropped_iterations` 0 and no failed
requests.

## Results

| Run | History | API p50 / p99 | Claim p50 / p99 | Sends/s | Drain time | Delivery lag p50 / p99 | API pool waits |
|---|---|---|---|---|---|---|---|
| A | none | | | | | | |
| B | 5 M | | | | | | |

Table sizes after seeding (from `seed.sql`):

## Why the numbers changed

_To write after both runs._
