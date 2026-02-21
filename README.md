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
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│            Service Layer                │
│   (Business logic, caching, batching)   │
└──────┬───────────────────────┬──────────┘
       │                       │
┌──────▼──────────┐  ┌────────▼─────────┐
│   Repository    │  │   Model Client   │
│  (Data Access)  │  │    (Scoring)     │
└──────┬──────────┘  └──────────────────┘
       │
┌──────▼─────────────────────────────────┐
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

## 14.3 Design Decisions

### Caching Strategy and TTL Rationale

The cache uses structured keys in the format `rec:user:{user_id}:limit:{limit}`, which means different limit values produce separate cache entries. This avoids the complexity of slicing a larger cached result while keeping cache logic simple.

The 10-minute TTL balances two competing concerns: freshness and performance. Recommendations don't need to update in real-time since users rarely watch multiple items within 10 minutes. Meanwhile, the TTL prevents stale data from persisting too long. When a user adds new watch history via `POST /users/{id}/watch-history`, the cache is explicitly invalidated using a pattern scan (`rec:user:{id}:limit:*`), ensuring immediate freshness when it matters.

Cache errors are logged but never propagated to the client. If Redis goes down, the service continues to function by hitting PostgreSQL directly, with degraded performance but no downtime.

### Concurrency Control Approach

The batch endpoint uses a channel-based semaphore with a buffer size of 10 to limit concurrent goroutines. Each goroutine must write a token to the channel before starting work and reads it back when finished. When all 10 slots are occupied, additional goroutines block until a slot opens.

The concurrency limit of 10 was chosen relative to the database connection pool size of 20. Each batch worker uses approximately 2 database connections during its lifecycle, so 10 workers consume roughly 20 connections at peak. This leaves headroom for single-user requests arriving simultaneously. A `sync.WaitGroup` tracks completion of all goroutines before aggregating results.

Individual user failures within a batch do not halt processing. Each goroutine captures its own error and records it in the results slice. The batch response includes a summary with success and failure counts, allowing the caller to identify and retry specific failures.

### Error Handling Philosophy

Errors are handled according to layer responsibility. The repository returns sentinel errors defined in the domain package (e.g., `domain.ErrUserNotFound`), the service propagates or wraps these errors with context using `fmt.Errorf("fetch user: %w", err)`, and the handler maps them to HTTP status codes using `errors.Is()`.

This separation means the service and repository have no knowledge of HTTP concepts, while the handler has no knowledge of database implementation. The domain package serves as the shared error vocabulary.

For the batch endpoint, per-user errors are captured as strings in the response. In a production system, these would be sanitized to avoid leaking internal details, but for this assessment the error messages are safe since they originate from known, controlled sources (user not found and model inference failures).

### Database Indexing Strategy

| Index | Purpose |
|-------|---------|
| `idx_users_country` | Supports potential geographic filtering of users |
| `idx_users_subscription` | Supports potential subscription-based content filtering |
| `idx_content_genre` | Enables fast filtering when querying candidates by genre |
| `idx_content_popularity` (DESC) | Optimizes the `ORDER BY popularity_score DESC LIMIT N` query used to fetch top candidates |
| `idx_watch_history_user` | Speeds up the JOIN when fetching a specific user's watch history |
| `idx_watch_history_content` | Supports the LEFT JOIN used in unwatched content filtering |
| `idx_watch_history_composite` (user_id, watched_at DESC) | Covers the common query pattern of fetching recent watch history ordered by time |

The composite index on `(user_id, watched_at DESC)` is particularly important because it allows PostgreSQL to satisfy both the `WHERE user_id = ?` filter and the `ORDER BY watched_at DESC` sort using a single index scan, avoiding a separate sort step.

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

```
execution: local
script: ./k6/single_user_load.js
output: -

scenarios: (100.00%) 1 scenario, 100 max VUs, 2m0s max duration (incl. graceful stop):
     * default: Up to 100 looping VUs for 1m30s over 3 stages (gracefulRampDown: 30s, gracefulStop: 30s)



█ THRESHOLDS

http_req_duration
✓ 'p(95)<500' p(95)=4.53ms

http_req_failed
✓ 'rate<0.05' rate=0.00%


█ TOTAL RESULTS

checks_total.......: 218684  2428.362884/s
checks_succeeded...: 100.00% 218684 out of 218684
checks_failed......: 0.00%   0 out of 218684

✓ status is 200 or 503
✓ has valid JSON body
✓ has recommendations on success
✓ has metadata

HTTP
http_req_duration..............: avg=1.71ms   min=200µs    med=1.19ms   max=65.06ms  p(90)=3.55ms   p(95)=4.53ms
{ expected_response:true }...: avg=1.71ms   min=200µs    med=1.19ms   max=65.06ms  p(90)=3.55ms   p(95)=4.53ms
http_req_failed................: 0.00%  0 out of 54671
http_reqs......................: 54671  607.090721/s

EXECUTION
iteration_duration.............: avg=102.54ms min=100.35ms med=102.17ms max=169.44ms p(90)=104.48ms p(95)=105.49ms
iterations.....................: 54671  607.090721/s
vus............................: 1      min=1          max=99
vus_max........................: 100    min=100        max=100

NETWORK
data_received..................: 79 MB  871 kB/s
data_sent......................: 5.6 MB 62 kB/s
```

#### Batch Endpoint Stress Test (30 VUs, 30 seconds)

```
execution: local
   script: ./k6/batch_stress.js
   output: -

scenarios: (100.00%) 1 scenario, 30 max VUs, 1m20s max duration (incl. graceful stop):
         * default: Up to 30 looping VUs for 50s over 3 stages (gracefulRampDown: 30s, gracefulStop: 30s)

█ THRESHOLDS

http_req_duration
✓ 'p(95)<5000' p(95)=4.78ms

http_req_failed
✓ 'rate<0.05' rate=0.00%


█ TOTAL RESULTS

checks_total.......: 6324    126.095824/s
checks_succeeded...: 100.00% 6324 out of 6324
checks_failed......: 0.00%   0 out of 6324

✓ status is 200
✓ has results array
✓ has summary
✓ has pagination info

HTTP
http_req_duration..............: avg=2.34ms   min=375µs    med=1.89ms   max=60.08ms  p(90)=3.9ms    p(95)=4.78ms
 { expected_response:true }...: avg=2.34ms   min=375µs    med=1.89ms   max=60.08ms  p(90)=3.9ms    p(95)=4.78ms
http_req_failed................: 0.00%  0 out of 1581
http_reqs......................: 1581   31.523956/s

EXECUTION
iteration_duration.............: avg=504.57ms min=500.54ms med=503.67ms max=562.64ms p(90)=507.86ms p(95)=510.17ms
iterations.....................: 1581   31.523956/s
vus............................: 2      min=1         max=29
vus_max........................: 30     min=30        max=30

NETWORK
data_received..................: 10 MB  205 kB/s
data_sent......................: 169 kB 3.4 kB/s
```

#### Cache Effectiveness Test

```
execution: local
   script: ./k6/cache_effectiveness.js
   output: -

scenarios: (100.00%) 1 scenario, 20 max VUs, 1m20s max duration (incl. graceful stop):
         * default: Up to 20 looping VUs for 50s over 3 stages (gracefulRampDown: 30s, gracefulStop: 30s)



█ THRESHOLDS

http_req_duration
✓ 'p(95)<500' p(95)=4.76ms

http_req_failed
✓ 'rate<0.05' rate=0.00%


█ TOTAL RESULTS

checks_total.......: 15249   304.670205/s
checks_succeeded...: 100.00% 15249 out of 15249
checks_failed......: 0.00%   0 out of 15249

✓ status is 200 or 503

CUSTOM
cache_hits.....................: 15249  304.670205/s

HTTP
http_req_duration..............: avg=2.08ms  min=219µs   med=1.76ms  max=15.22ms p(90)=3.84ms p(95)=4.76ms
 { expected_response:true }...: avg=2.08ms  min=219µs   med=1.76ms  max=15.22ms p(90)=3.84ms p(95)=4.76ms
http_req_failed................: 0.00%  0 out of 15249
http_reqs......................: 15249  304.670205/s

EXECUTION
iteration_duration.............: avg=52.78ms min=50.38ms med=52.52ms max=69.92ms p(90)=54.6ms p(95)=55.66ms
iterations.....................: 15249  304.670205/s
vus............................: 1      min=1          max=20
vus_max........................: 20     min=20         max=20

NETWORK
data_received..................: 18 MB  362 kB/s
data_sent......................: 1.6 MB 31 kB/s
```

### Identified Bottlenecks and Limiting Factors

The primary bottleneck is the **simulated model latency** of 30-50ms per user. This is an artificial constraint that dominates single-request response time. Without it, cache-miss requests would complete in under 10ms based on database query time alone.

For the batch endpoint, throughput is limited by `batchConcurrency × model_latency`. With 10 workers and ~40ms average model latency, processing 20 users takes approximately `(20/10) × 40ms = 80ms` for model scoring plus database query time, totaling ~200-400ms per batch request.

The **database connection pool** (20 max connections) is the second constraint. Under heavy concurrent load, goroutines may block waiting for a connection. This is intentional — unbounded connections would overwhelm PostgreSQL.

### Cache Hit Rate Analysis

The cache effectiveness test demonstrates a hit rate above 99% after the initial warm-up period. With 5 unique cache keys and a 10-minute TTL, the first 5 requests are cache misses (one per user), and all subsequent requests are served directly from Redis in under 5ms. This reduces average response time by approximately 90% compared to uncached requests and eliminates database load for repeat requests.

---

## 14.5 Trade-offs and Future Improvements

### Known Limitations

**No cache warming.** The first request for each user always experiences the full pipeline latency. In production, a background job could pre-populate the cache for active users during off-peak hours.

**Pattern-based cache invalidation.** The `SCAN` command used to find and delete cache keys for a user is O(N) in total Redis keys. With millions of users, this becomes a bottleneck. An alternative is maintaining a Redis Set of cache keys per user for O(1) invalidation.

**In-process seeding.** The seed logic is bundled with the application binary. In a production system, seeding and migrations should be separate CLI commands or init containers.

**No rate limiting.** The service has no protection against abusive traffic. A token bucket middleware or API gateway would be necessary in production.

### Scalability Considerations

**Horizontal scaling.** The service is stateless — all shared state lives in PostgreSQL and Redis. It can scale to N instances behind a load balancer with no code changes.

**Database read replicas.** The repository could accept a separate read-only connection pool pointed at a PostgreSQL replica, offloading recommendation queries from the primary.

**Redis Cluster.** For large user bases, a single Redis instance may not have enough memory. Redis Cluster provides automatic sharding across multiple nodes.

**Asynchronous batch processing.** For very large user sets, the batch endpoint could publish user IDs to a message queue (e.g., RabbitMQ, Kafka) and process them asynchronously, returning a job ID instead of blocking.

### Proposed Enhancements

**Circuit breaker** on the model client to fast-fail during sustained outages instead of waiting for each request to timeout individually.

**Request coalescing** to deduplicate concurrent requests for the same user and limit combination. If 10 requests arrive for user 1 simultaneously, only one hits the database and the rest wait for the cached result.

**A/B testing support** with versioned scoring algorithms and traffic splitting, enabling experimentation with different weight configurations.

**Observability** with Prometheus metrics for latency histograms, cache hit rates, error rates by type, and database connection pool utilization. Combined with Grafana dashboards for operational visibility.

**Structured logging** replacing `log.Printf` with a structured logger (e.g., `slog` or `zerolog`) that outputs JSON logs with request IDs, user IDs, and latency measurements for easier debugging and log aggregation.

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

## Stopping the Application

```bash
docker-compose down       # stop containers, keep data
docker-compose down -v    # stop containers, delete all data
```