# Lab 01 — worker contention

## Question

Three worker loops poll the same queue with a plain `SELECT … LIMIT`. How many
deliveries reach the receiver more than once?

## Prediction (written before the first run)

Close to half of the 12,000 messages delivered twice.

## Setup

| | |
|---|---|
| Load | k6 `constant-arrival-rate`, 200 req/s for 60 s → 12,000 messages |
| Tenant | one tenant, one endpoint |
| Worker | one process, `WORKER_COUNT=3`, `WORKER_BATCH_SIZE=10` |
| Receiver | `cmd/receiver -latency 50ms`, no failures |
| Claim | `WORKER_CLAIM_MODE`: `naive` (`ListDueDeliveries`) or `skiplocked` (`ClaimDueDeliveries`), both in `postgres/queries/delivery.sql` |
| Database | `webhook_lab_bench`, reset before each run |
| Machine | one laptop runs everything: compare runs, do not trust absolute numbers |

## The two claim strategies

**naive** reads due rows and hands them out. Nothing marks a row as taken, so
every loop that reads it sends it.

**skiplocked** takes and marks rows in one statement:

```sql
WITH due AS (
    SELECT id FROM deliveries
    WHERE status = 'pending' AND next_attempt_at <= now()
    ORDER BY next_attempt_at LIMIT @batch_size
    FOR UPDATE SKIP LOCKED          -- concurrent claims pass over each other's rows
), claimed AS (
    UPDATE deliveries d
    SET next_attempt_at = now() + @lease   -- lease: hidden from other workers
    FROM due WHERE d.id = due.id
    RETURNING ...
)
SELECT ... FROM claimed JOIN endpoints ... JOIN messages ...
```

`SKIP LOCKED` alone would not be enough here. The row locks end when the claim
statement commits, before any HTTP request is sent; the lease on
`next_attempt_at` is what keeps other loops away during the send. If a worker
dies mid-batch, the lease runs out and the delivery becomes due again, so a
crash can still cause a resend: at-least-once, not exactly-once.

`WORKER_CLAIM_LEASE` (default 2 m) must exceed `WORKER_BATCH_SIZE` ×
`DELIVERY_TIMEOUT`; the worker refuses to start otherwise.

What to compare on the database side:

- `delivery_claim_duration_seconds`: the claim query alone. `skiplocked`
  writes (row locks and the lease update), `naive` only reads, so its claim
  should be faster.
- attempts and updates in total: `naive` writes about three times as many.

## Run

Each process in its own terminal, from the repo root. `POSTGRES_URL` overrides
`.env`, so the API and worker use the bench database.

```bash
export POSTGRES_URL='postgres://localhost:5432/webhook_lab_bench?sslmode=disable'

go run ./cmd/receiver -latency 50ms       # terminal 1, restart before every run
go run ./cmd/api                          # terminal 2
WORKER_COUNT=3 WORKER_CLAIM_MODE=naive go run ./cmd/worker        # terminal 3, first run
WORKER_COUNT=3 WORKER_CLAIM_MODE=skiplocked go run ./cmd/worker   # terminal 3, second run

labs/01-worker-contention/setup.sh        # terminal 4
source labs/01-worker-contention/.lab.env
k6 run -e API_KEY="$API_KEY" labs/01-worker-contention/load.js
labs/01-worker-contention/report.sh
```

A run counts only when k6 reports `dropped_iterations` 0 and no failed
requests; the thresholds fail the run otherwise.

## Results

Run 2 is the valid naive run. Run 1 is kept below as a note: it dropped 239
iterations, so it does not count.

| Claim | Messages | Received | Duplicates | Sends per delivery | Drain time | Delivery lag p50 / p99 | API p99 | API pool acquire waits |
|---|---|---|---|---|---|---|---|---|
| plain `SELECT` | 12,000 | 35,923 | 23,923 | 2.99 | 657 s | 302 s / 594 s | 803 ms | 1,953 |
| `FOR UPDATE SKIP LOCKED` + lease | 12,000 | 12,000 | 0 | 1.00 | 218 s | 79 s / 292 s | 12.5 ms | 7 |

- Receiver and database agree: 23,923 extra successful sends on both sides;
  11,994 of 12,000 deliveries (99.95%) were sent more than once.
- Delivery throughput was about 54 sends/s across the three loops, but only a
  third of those were new deliveries, so the queue drained at roughly the rate
  of one loop. The due backlog peaked at 10,742.
- The load lasted 60 s; draining took 657 s.
- With `skiplocked`, receiver and database agree on 0 extra sends. The loops
  sent about 52 times a second, the same as naive, but every send was a new
  delivery, so the queue drained three times faster: 218 s against 657 s, and
  the due backlog peaked at 8,430 instead of 10,742.
- The backlog still grew: 3 loops at ~55 ms per send cannot keep up with
  200 messages/s. Keeping up needs about 200 × 0.055 ≈ 11 loops.

### Claim query cost on the database

Measured once each with `EXPLAIN (ANALYZE, BUFFERS)` against the same 12,000
pending deliveries, one session, no concurrency, inside a transaction that was
rolled back:

| | Execution | Shared buffers touched | Writes |
|---|---|---|---|
| naive `SELECT … LIMIT 10` | 0.12 ms | ~230 | none |
| `skiplocked` claim of 10 | 1.55 ms | ~420 | 10 row locks, 10 row updates |

Seen from the worker, the claim took 2.5 ms on average (p50 2.2 ms, p99 16 ms)
over 1,278 claims during the run; that includes the round trip and waiting for
a pool connection. The naive run predates this metric.

The claim costs more because it writes: `FOR UPDATE` marks each row, and the
lease is an `UPDATE`. That write touches `next_attempt_at`, which is in the
partial index `deliveries_due_idx`, so Postgres cannot use a HOT update and
must add an index entry for each claimed row. Over a whole run the
skiplocked path still writes less: each delivery row is updated twice (lease,
then outcome) against three times under naive (one outcome per loop), and
attempts drop from 35,923 to 12,000.

### Notes

- **Prediction vs result.** The prediction was about half of the messages
  sent twice. The result is every delivery sent three times: one send per
  loop, because no loop marks a row as taken.
- **API p99 fell from 803 ms to 12.5 ms** and API pool acquire waits from
  1,953 to 7, though the API code did not change. The API never touches the
  rows the worker claims, so this is not lock contention on `deliveries`. The
  likely cause is total database and CPU load: naive wrote three times the
  attempts and outcome updates on the same laptop. Not proven; the worker-off
  run would settle it.
- **API latency is not caused by the receiver.** `POST /messages` never calls
  the receiver. The API pool (8 connections) recorded 1,953 acquire waits
  while the worker pool recorded none. Not yet proven: whether the worker's
  load on Postgres or the API pool size is the cause. Next check: the same k6
  run with the worker stopped.
- **Lag p99 is an estimate.** `delivery_lag_seconds` has buckets at 120 s and
  300 s, and both p99 values land between them, so Prometheus interpolates.
  The p50 values and the drain times are the firmer comparison.
- **Grafana's p99 reads higher than k6's.** The dashboard showed 1.73 s at
  19:26 while k6 measured p99 1.04 s and max 1.31 s for run 1.
  `histogram_quantile` interpolates inside the bucket it lands in, here 1 s to
  2.5 s, so it can report a value above the real maximum. k6's numbers come
  from raw samples and are the ones to quote.
- **Run 1 (invalid).** 239 dropped iterations while k6 added VUs mid-run
  (50 preallocated, peaked at 155). Same shape: 11,762 messages, 35,157
  received, 2.99 sends per delivery, 661 s to drain. `load.js` now
  preallocates 300 VUs.

## Prediction for skiplocked (written before the run)

No prediction from Sony. Claude's estimate from queueing arithmetic: 0
duplicates; 3 loops at ~55 ms per send give ~55–60 deliveries/s, below the
200/s arriving, so the backlog still grows and draining 12,000 takes ~200 s.

## Why the number changed

Under the naive claim, nothing marked a row as taken, so each of the three
loops read the same due rows and sent them: every delivery went out about
three times, and two thirds of the worker's capacity was spent on repeats. The
skiplocked claim takes rows and pushes their `next_attempt_at` forward in the
same statement. `FOR UPDATE SKIP LOCKED` keeps two concurrent claims from
picking the same rows, and the lease keeps them hidden while the HTTP request
runs, after the row locks are gone. The worker sent at the same rate in both
runs, about 52 a second, but now every send was a new delivery, so the queue
drained three times faster. The claim query itself got slower, 0.12 ms to
1.55 ms for ten rows, because it writes; per run the database did less work.

## Open questions

- How does the claim behave when `deliveries` holds millions of finished rows
  and a large pending backlog? Every run here started from an empty table.
  The partial index should keep the claim cheap, but lease updates are not HOT
  and leave dead index entries behind. This belongs with the lab on bloat and
  data retention.
- Is the API's p99 drop caused by database load from the worker? Check with the
  same k6 run and the worker stopped.
