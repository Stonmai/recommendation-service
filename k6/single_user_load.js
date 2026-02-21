import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
    stages: [
        { duration: '15s', target: 50 },   // ramp up
        { duration: '1m', target: 100 },    // sustained load
        { duration: '15s', target: 0 },     // ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'],   // 95% of requests under 500ms
        http_req_failed: ['rate<0.05'],     // less than 5% failures (accounts for simulated model failures)
    },
};

export default function () {
    const userId = Math.floor(Math.random() * 20) + 1;
    const limit = [5, 10, 15, 20][Math.floor(Math.random() * 4)];

    const res = http.get(
        `http://localhost:8080/users/${userId}/recommendations?limit=${limit}`
    );

    check(res, {
        'status is 200 or 503': (r) => r.status === 200 || r.status === 503,
        'has valid JSON body': (r) => {
            try {
                JSON.parse(r.body);
                return true;
            } catch (e) {
                return false;
            }
        },
        'has recommendations on success': (r) => {
            if (r.status === 200) {
                const body = JSON.parse(r.body);
                return body.recommendations && body.recommendations.length > 0;
            }
            return true; // skip check for non-200
        },
        'has metadata': (r) => {
            if (r.status === 200) {
                const body = JSON.parse(r.body);
                return body.metadata && body.metadata.hasOwnProperty('cache_hit');
            }
            return true;
        },
    });

    sleep(0.1);
}
