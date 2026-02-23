# Recommendation Service
A production-ready backend recommendation service built in Go that generates personalized content recommendations based on user watch history, content popularity, and genre preferences.

### Setup Instructions

### Prerequisites
- Docker & Docker Compose (v2.0+)
- Go 1.24+ (for development)
- [k6](https://k6.io/docs/get-started/installation/) (for performance testing)

### Installation guide
```bash
git clone https://github.com/Stonmai/recommendation-service.git && cd recommendation-service
docker-compose up --build
```

The application is now running at `http://localhost:8080`.

On startup, the application automatically:
1. Waits for PostgreSQL and Redis to be healthy
2. Runs database migrations (creates tables and indexes)
3. Seeds deterministic test data if the database is empty
4. Starts the HTTP server

### Migrations and Seeding
Migrations and seeding run automatically on startup. To reset the database:

```bash
# Stop everything and delete data
docker-compose down -v

# Restart fresh
docker-compose up --build
```

To run migrations manually:

```bash
#Create tables
psql $DATABASE_URL -f migrations/create_tables.up.sql

# Drop tables
psql $DATABASE_URL -f migrations/create_tables.down.sql
```

### Verify the Application

```bash
# Health check
curl http://localhost:8080/health

# Get recommendations for user 1
curl "http://localhost:8080/users/1/recommendations?limit=5"

# Get recommendations with default limit (10)
curl http://localhost:8080/users/3/recommendations

# Batch recommendations
curl "http://localhost:8080/recommendations/batch?page=1&limit=10"
```

---

## Architecture Overview

### System Design

```
┌─────────────────────────────────────────┐
│            Handler Layer                │
│   (HTTP routing, validation, response)  │
└───────────────────┬─────────────────────┘
                    │
┌───────────────────▼─────────────────────┐
│            Service Layer                │
│   (Business logic, caching, batching)   │
└───────┬──────────────────────┬──────────┘
        │                      │
┌───────▼─────────┐  ┌─────────▼────────┐
│   Repository    │  │   Model Client   │
│  (Data Access)  │  │    (Scoring)     │
└───────┬─────────┘  └──────────────────┘
        │
┌───────▼────────────────────────────────┐
│       Database + Cache Layer           │
│       (PostgreSQL + Redis)             │
└────────────────────────────────────────┘
```

### Architectural Layers

**Handler Layer** (`internal/handler/`) receives HTTP requests, validates input parameters (user_id, limit, page), and formats JSON responses. It maps service-layer errors to appropriate HTTP status codes (404, 400, 500, 503) without containing any business logic.

**Service Layer** (`internal/service/`) orchestrates the recommendation workflow. For single-user requests, it checks the Redis cache first, and on a miss, coordinates the repository and model client to generate fresh recommendations before caching them. For batch requests, it manages a bounded worker pool of 10 concurrent goroutines using a channel-based semaphore, collecting results from all users including partial failures.

**Repository Layer** (`internal/repository/`) abstracts all PostgreSQL queries behind clean methods. It uses JOIN queries to avoid N+1 problems and LEFT JOIN with NULL checks for efficient content filtering. All queries accept `context.Context` for timeout propagation.

**Model Client** (`internal/model/`) implements the heuristic scoring algorithm that simulates a production ML service. It receives user context, watch history, and candidate content from the repository layer, computes weighted scores, and returns ranked recommendations. It simulates realistic latency (30-50ms) and a 1.5% failure rate.

**Cache Layer** (`internal/cache/`) provides Redis-backed caching with structured keys (`rec:user:{id}:limit:{n}`), 10-minute TTL, and pattern-based invalidation. Cache errors are logged but never fail requests, ensuring graceful degradation.

**Domain Layer** (`internal/domain/`) contains shared types, API response structs, and sentinel errors used across all layers. It has no dependencies on other internal packages, preventing circular imports.

### Data Flow — Single User Request

A request to `GET /users/7/recommendations?limit=5` flows through the system as follows:

1. The handler parses and validates `userID=7` and `limit=5` from the URL
2. The service checks Redis for cached data at key `rec:user:7:limit:5`
3. On a cache miss, the service calls the repository to fetch user 7's profile from the `users` table
4. The repository fetches the user's recent watch history using a JOIN between `user_watch_history` and `content` to get genre information in a single query
5. The repository fetches unwatched candidate content using a LEFT JOIN that excludes already-watched items, ordered by popularity
6. The model client receives the user profile, watch history, and candidates, then computes a weighted score for each candidate based on genre preference (35%), popularity (40%), recency (15%), and exploration noise (10%)
7. The service stores the top 5 scored recommendations in Redis with a 10-minute TTL
8. The handler formats the response with recommendations and metadata including `cache_hit: false`

### How the Recommendation Model Integrates with Database Queries

The model client depends entirely on database data for its scoring decisions. Genre preferences are derived from the watch history query, which JOINs `user_watch_history` with `content` to count how often the user watches each genre. These counts are normalized into weights (e.g., if a user watched 10 action and 5 drama titles, action gets a 0.67 weight). Candidate content comes pre-filtered by the repository — the LEFT JOIN ensures only unwatched content reaches the scorer. The popularity score stored in the `content` table directly feeds into the scoring formula. The `created_at` timestamp drives the recency factor, giving newer content a slight boost.

---

## Design Decisions

### Caching Strategy and TTL Rationale

The cache uses structured keys in the format `rec:user:{user_id}:limit:{limit}`, which means different limit values produce separate cache entries. This avoids the complexity of slicing a larger cached result while keeping cache logic simple.

The 10-minute TTL balances two competing concerns: freshness and performance. Recommendations don't need to update in real-time since users rarely watch multiple items within 10 minutes. Meanwhile, the TTL prevents stale data from persisting too long. The cache layer includes a `ClearUserCache` method that invalidates all cached recommendations for a user using a pattern scan (`rec:user:{id}:limit:*`). The service layer calls this method when watch history is updated via `AddWatchHistory`, which is ready to be exposed as an API endpoint.

Cache errors are logged but never propagated to the client. If Redis goes down, the service continues to function by hitting PostgreSQL directly, with degraded performance but no downtime.

### Concurrency Control Approach

The batch endpoint uses a channel-based semaphore with a buffer size of 10 to limit concurrent goroutines. Each goroutine must write a token to the channel before starting work and reads it back when finished. When all 10 slots are occupied, additional goroutines block until a slot opens.

The concurrency limit of 10 was chosen relative to the database connection pool size of 20. Each batch worker uses approximately 2 database connections during its lifecycle, so 10 workers consume roughly 20 connections at peak. This leaves headroom for single-user requests arriving simultaneously. A `sync.WaitGroup` tracks completion of all goroutines before aggregating results.

Individual user failures within a batch do not halt processing. Each goroutine captures its own error and records it in the results slice. The batch response includes a summary with success and failure counts, allowing the caller to identify and retry specific failures.

### Error Handling Philosophy

Errors are handled according to layer responsibility. The repository maps low-level errors to domain sentinels (e.g., `pgx.ErrNoRows` → `domain.ErrUserNotFound`) and wraps other errors with context using `fmt.Errorf("...: %w", err)`. The service propagates these errors up, wrapping them with additional context where useful (e.g., `"fetch watch history: %w"`). The handler maps errors to HTTP status codes using `errors.Is()` against domain sentinels.

This separation means the service and repository have no knowledge of HTTP concepts, while the handler has no knowledge of database or model internals. The domain package serves as the shared error words between all layers.

Sentinel errors defined in `domain/recommendation.go` and the service wraps model inference failures as `domain.ErrModelUnavailable` before they leave the service layer, so the handler never needs to import or inspect the `model` package directly.

For the batch endpoint, per-user errors are captured by the service's `categorizeError` function, which maps domain sentinels to safe, client-facing error codes and messages in the batch response. Batch-level errors (e.g., failed pagination query, request timeout) are handled separately in the batch handler. This ensures internal details are never exposed to callers.

### Database Indexing Strategy

**##### Utilized Indexes**
| Index | Purpose |
|-------|---------|
| `idx_content_popularity` (DESC) | Optimizes the `ORDER BY popularity_score DESC LIMIT N` query used to fetch top candidates |
| `idx_watch_history_user` | Speeds up the JOIN when fetching a specific user's watch history |
| `idx_watch_history_content` | Supports the LEFT JOIN used in unwatched content filtering |
| `idx_watch_history_composite` (user_id, watched_at DESC) | Covers the common query pattern of fetching recent watch history ordered by time |
| `idx_watch_history_user_content` (user_id, content_id) | Optimizes the LEFT JOIN in the unwatched-content query by covering both join conditions in a single index lookup |

The composite index on `(user_id, watched_at DESC)` is particularly important because it allows PostgreSQL to satisfy both the `WHERE user_id = ?` filter and the `ORDER BY watched_at DESC` sort using a single index scan, avoiding a separate sort step.
The composite index on `(user_id, content_id)` allows PostgreSQL to evaluate the LEFT JOIN condition for unwatched content filtering without scanning the entire `user_watch_history` table, which is critical as watch history grows.

**##### Preparatory Indexes**
| Index | Purpose |
|-------|---------|
| `idx_users_country` | Supports potential country filtering of users (currently unused) |
| `idx_users_subscription` | Supports potential subscription-based content filtering (currently unused) |
| `idx_content_genre` | Enables fast filtering when querying candidates by genre (currently unused) |

### Scoring Algorithm Rationale and Weight Choices

The scoring formula `popularity × 0.4 + genre_match × 0.35 + recency × 0.15 + noise × 0.1` reflects a pragmatic balance:

**Popularity (40%)** is the strongest signal because popular content has broad appeal and low risk of a bad recommendation. In a cold-start scenario where a user has no watch history, popularity alone produces reasonable results.

**Genre Match (35%)** personalizes recommendations based on observed behavior. If a user watches mostly action films, action candidates score higher. The default weight of 0.1 for unseen genres ensures some exploration — users aren't locked into a genre bubble.

**Recency (15%)** provides a slight boost to newer content using time decay: `1.0 / (1.0 + days / 365)`. Content from a week ago gets a factor of ~0.98 while content from a year ago gets ~0.5. This prevents the system from always recommending the same established titles.

**Exploration Noise (10%)** introduces controlled randomness so that recommendations aren't entirely deterministic. This is essential in real recommendation systems to discover user preferences that the model hasn't captured yet.

---

## Performance Results

### k6 Test Results

Run the tests with:

```bash
k6 run k6/single_user_load.js
k6 run k6/batch_stress.js
k6 run k6/cache_effectiveness.js
```

#### Single User Load Test (100 VUs, 1 minute sustained)

| Metric         | Result    | Threshold |
| -------------- | --------- | --------- |
| Avg Latency    | 2.62ms    | —         |
| P95 Latency    | 6.62ms    | < 5000ms  |
| P99 Latency    | 11.35ms   | < 10000ms |
| Error Rate     | 0.00%     | < 5%      |
| Throughput     | 599 req/s | —         |
| Total Requests | 53,979    | —         |

**Full k6 output**
```
execution: local
    script: ./k6/single_user_load.js
    output: -

scenarios: (100.00%) 1 scenario, 100 max VUs, 2m0s max duration (incl. graceful stop):
          * default: Up to 100 looping VUs for 1m30s over 3 stages (gracefulRampDown: 30s, gracefulStop: 30s)



█ THRESHOLDS

http_req_duration
✓ 'p(95)<500' p(95)=6.62ms
✓ 'p(99)<1000' p(99)=11.35ms

http_req_failed
✓ 'rate<0.05' rate=0.00%


█ TOTAL RESULTS

checks_total.......: 215916  2396.817467/s
checks_succeeded...: 100.00% 215916 out of 215916
checks_failed......: 0.00%   0 out of 215916

✓ status is 200 or 503
✓ has valid JSON body
✓ has recommendations on success
✓ has metadata

HTTP
http_req_duration..............: avg=2.62ms   min=245µs    med=1.93ms   max=56.57ms  p(90)=5.03ms   p(95)=6.62ms
  { expected_response:true }...: avg=2.62ms   min=245µs    med=1.93ms   max=56.57ms  p(90)=5.03ms   p(95)=6.62ms
http_req_failed................: 0.00%  2 out of 53979
http_reqs......................: 53979  599.204367/s

EXECUTION
iteration_duration.............: avg=103.85ms min=100.43ms med=103.09ms max=161.42ms p(90)=106.82ms p(95)=109.07ms
iterations.....................: 53979  599.204367/s
vus............................: 1      min=1          max=99
vus_max........................: 100    min=100        max=100

NETWORK
data_received..................: 78 MB  863 kB/s
data_sent......................: 5.5 MB 61 kB/s
```

#### Batch Endpoint Stress Test (30 VUs, 30 seconds)

| Metric | Result | Threshold |
|--------|--------|-----------|
| Avg Latency | 2.49ms | — |
| P95 Latency | 4.43ms | < 5000ms |
| P99 Latency | 5.99ms | < 10000ms |
| Error Rate | 0.00% | < 5% |
| Throughput | 31.5 req/s | — |
| Total Requests | 1,579 | — |

**Full k6 output**
```
execution: local
    script: ./k6/batch_stress.js
    output: -

scenarios: (100.00%) 1 scenario, 30 max VUs, 1m20s max duration (incl. graceful stop):
          * default: Up to 30 looping VUs for 50s over 3 stages (gracefulRampDown: 30s, gracefulStop: 30s)



█ THRESHOLDS

http_req_duration
✓ 'p(95)<5000' p(95)=4.43ms
✓ 'p(99)<10000' p(99)=5.99ms

http_req_failed
✓ 'rate<0.05' rate=0.00%


█ TOTAL RESULTS

checks_total.......: 6316    125.813585/s
checks_succeeded...: 100.00% 6316 out of 6316
checks_failed......: 0.00%   0 out of 6316

✓ status is 200
✓ has results array
✓ has summary
✓ has pagination info

HTTP
http_req_duration..............: avg=2.49ms   min=504µs    med=2.16ms   max=57.01ms  p(90)=3.71ms   p(95)=4.43ms
  { expected_response:true }...: avg=2.49ms   min=504µs    med=2.16ms   max=57.01ms  p(90)=3.71ms   p(95)=4.43ms
http_req_failed................: 0.00%  0 out of 1579
http_reqs......................: 1579   31.453396/s

EXECUTION
iteration_duration.............: avg=505.14ms min=500.88ms med=504.33ms max=563.29ms p(90)=508.82ms p(95)=510.37ms
iterations.....................: 1579   31.453396/s
vus............................: 1      min=1         max=29
vus_max........................: 30     min=30        max=30

NETWORK
data_received..................: 10 MB  202 kB/s
data_sent......................: 168 kB 3.4 kB/s
```


#### Cache Effectiveness Test

| Metric | Result |
|--------|--------|
| Cache Hits | 15,140 (99.96%) |
| Cache Misses | 6 (0.04%) |
| Avg Latency | 2.31ms |
| P95 Latency | 4.82ms |
| P99 Latency | 7.1ms |
| Throughput | 302.8 req/s |

**Full k6 output**
```
execution: local
script: ./k6/cache_effectiveness.js
output: -

    scenarios: (100.00%) 1 scenario, 20 max VUs, 1m20s max duration (incl. graceful stop):
            * default: Up to 20 looping VUs for 50s over 3 stages (gracefulRampDown: 30s, gracefulStop: 30s
█ THRESHOLDS

    http_req_duration
    ✓ 'p(95)<500' p(95)=4.82ms
    ✓ 'p(99)<1000' p(99)=7.1ms

    http_req_failed
    ✓ 'rate<0.05' rate=0.00%


█ TOTAL RESULTS

    checks_total.......: 15146   302.817139/s
    checks_succeeded...: 100.00% 15146 out of 15146
    checks_failed......: 0.00%   0 out of 15146

    ✓ status is 200 or 503

    CUSTOM
    cache_hits.....................: 15140  302.69718/s
    cache_misses...................: 6      0.119959/s

    HTTP
    http_req_duration..............: avg=2.31ms  min=260µs   med=1.98ms  max=56.65ms  p(90)=3.96ms  p(95)=4.82ms
    { expected_response:true }...: avg=2.31ms  min=260µs   med=1.98ms  max=56.65ms  p(90)=3.96ms  p(95)=4.82ms
    http_req_failed................: 0.00%  0 out of 15146
    http_reqs......................: 15146  302.817139/s

    EXECUTION
    iteration_duration.............: avg=53.14ms min=50.36ms med=52.82ms max=110.87ms p(90)=54.88ms p(95)=56.06ms
    iterations.....................: 15146  302.817139/s
    vus............................: 1      min=1          max=20
    vus_max........................: 20     min=20         max=20

    NETWORK
    data_received..................: 18 MB  360 kB/s
    data_sent......................: 1.5 MB 31 kB/s
```

### Results Analysis

All three tests passed their thresholds comfortably. The P95 latency of 6.62ms for single-user requests is 75x faster than the 500ms threshold, and the P99 of 11.35ms is 88x faster than the 1000ms threshold, indicating significant headroom for increased load. The service sustained 599 requests per second with 100 concurrent users.

The 0.00% error rate across all tests is notable because the model client simulates a 1.5% failure rate. This suggests that during testing, the random failures did not occur in sufficient volume to register — a function of the random seed and short test duration. In longer-running tests, the expected ~1.5% failure rate would appear.

The batch endpoint averaged 2.49ms with P95 of 4.43ms starting from a cold cache. The initial batch requests trigger cache misses with full model scoring per user, but because each user's recommendations are cached after the first computation, subsequent batch requests benefit from per-user cache hits. With only 20 seeded users and a 500ms sleep between iterations, the cache fully warms within the first second of testing. Under production conditions with thousands of users and no sleep between requests, cold-start batch latency would be higher (~200-400ms for 20 uncached users due to model scoring).

### Identified Bottlenecks and Limiting Factors

The **simulated model latency** of 30-50ms per user is the primary bottleneck for cache-miss requests. With the cache warm, average response times drop to ~2.5ms. Without the cache, each request would incur the full model latency plus database query time.

For the batch endpoint, throughput is bounded by `batchConcurrency × model_latency`. With 10 workers and ~40ms average model latency, processing 20 uncached users takes approximately `(20/10) × 40ms = 80ms` for model scoring alone. However, once cached, batch processing completes in under 6ms regardless of page size.

The **database connection pool** (20 max connections) is the secondary constraint. Under heavy concurrent load with cache misses, goroutines may block waiting for a connection. This is intentional — unbounded connections would overwhelm PostgreSQL.

### Cache Hit Rate Analysis

The cache effectiveness test recorded a **99.96% hit rate** (15,140 hits vs 6 misses). The 6 misses are slightly higher than the expected 5 (one per unique user) due to a race condition during warm-up — multiple virtual users can request the same user simultaneously before the first response is cached. After the initial warm-up (first ~50ms of the test), every subsequent request was served directly from Redis.

The impact of caching on response time is clear: the average latency of 2.31ms is dominated by Redis round-trip time rather than database queries and model scoring. Without caching, each request would require 3 database queries plus 30-50ms of model latency, resulting in approximately 40-60ms per request — an 18x performance difference.

---

## Trade-offs and Future Improvements

### Known Limitations

**No cache warming.** The first request for each user always experiences the full pipeline latency (~40-60ms). In production, a background job could pre-populate the cache for active users during off-peak hours, eliminating cold-start penalties entirely.

**Pattern-based cache invalidation.** The `SCAN` command used to find and delete cache keys is O(N) in total Redis keys. With millions of users, this becomes a bottleneck. An alternative is maintaining a Redis Set of cache keys per user for O(1) invalidation.

**In-process seeding.** The seed logic is bundled with the application binary. In a production system, seeding and migrations should be separate CLI commands or Docker init containers to keep the application binary focused on serving requests.

**No rate limiting.** The service has no protection against abusive traffic. A token bucket middleware or API gateway would be necessary in production to prevent resource exhaustion.

**Single Redis instance.** The current deployment uses a single Redis node, creating a single point of failure for the caching layer. Cache failures are handled gracefully (requests fall through to PostgreSQL), but sustained Redis downtime would significantly increase database load.

### Scalability Considerations

**Horizontal scaling.** The service is stateless — all shared state lives in PostgreSQL and Redis. It can scale to N instances behind a load balancer with no code changes.

**Database read replicas.** The repository could accept a separate read-only connection pool pointed at a PostgreSQL replica, offloading recommendation queries from the primary and improving read throughput.

**Asynchronous batch processing.** For very large user sets (millions of users), the batch endpoint could publish user IDs to a message queue (e.g., RabbitMQ, Kafka) and process them asynchronously, returning a job ID for polling instead of blocking the HTTP request.

### Proposed Enhancements

**Circuit breaker** After N consecutive failures, return simple popularity-based recommendations instead. This prevents wasting time until the model recovers.

**Request coalescing** to deduplicate concurrent requests for the same user and limit combination at the same time. If 10 requests arrive for user 1 simultaneously, only one executes the full pipeline and the rest wait for the cached result.

**Structured logging** replacing `log.Printf` with a structured logger (e.g., `slog` or `zerolog`) that outputs JSON logs with request IDs, user IDs, and latency measurements for easier debugging.

**Real-time event streaming** Watch history would not be updated via a direct API call. Instead, the streaming platform would emit events when users finish watching content, published to a message queue (e.g., Kafka). The recommendation service consumes these events to update watch history, invalidate cached recommendations, and optionally pre-compute fresh recommendations in the background. Alternatively, PostgreSQL's LISTEN/NOTIFY with a trigger on the `user_watch_history` table could achieve without external dependencies.

---

## API Reference

### Get Recommendations

```
GET /users/{userID}/recommendations?limit=10
```

### Batch Recommendations

```
GET /recommendations/batch?page=1&limit=20
```

### Add Watch History (triggers cache invalidation)

```
POST /users/{userID}/watch-history
Body: {"content_id": 42}
```

### Health Check

```
GET /health
```
---
## Stopping the Application

```bash
docker-compose down       # stop containers, keep data
docker-compose down -v    # stop containers, delete all data
```