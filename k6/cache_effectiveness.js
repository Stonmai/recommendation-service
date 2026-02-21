import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const cacheHits = new Counter('cache_hits');
const cacheMisses = new Counter('cache_misses');

export let options = {
    // Lower concurrency to clearly observe cache behavior
    stages: [
        { duration: '10s', target: 20 },
        { duration: '30s', target: 20 },
        { duration: '10s', target: 0 },
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'],
        http_req_failed: ['rate<0.05'],
    },
};

export default function () {
    // Use a small set of user IDs to maximize cache hits
    const userId = Math.floor(Math.random() * 5) + 1;  // only users 1-5
    const limit = 10;  // fixed limit for consistent cache keys

    const res = http.get(
        `http://localhost:8080/users/${userId}/recommendations?limit=${limit}`
    );

    check(res, {
        'status is 200 or 503': (r) => r.status === 200 || r.status === 503,
    });

    if (res.status === 200) {
        try {
            const body = JSON.parse(res.body);
            if (body.metadata && body.metadata.cache_hit === true) {
                cacheHits.add(1);
            } else {
                cacheMisses.add(1);
            }
        } catch (e) {
            // ignore parse errors
        }
    }

    sleep(0.05);  // short sleep to generate rapid requests
}
