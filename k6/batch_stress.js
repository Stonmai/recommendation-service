import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
    stages: [
        { duration: '10s', target: 10 },
        { duration: '30s', target: 30 },
        { duration: '10s', target: 0 },
    ],
    thresholds: {
        http_req_duration: ['p(95)<5000'],
        http_req_failed: ['rate<0.05'],
    },
};

export default function () {
    const page = Math.floor(Math.random() * 3) + 1;
    const limit = [5, 10, 20][Math.floor(Math.random() * 3)];

    const res = http.get(
        `http://localhost:8080/recommendations/batch?page=${page}&limit=${limit}`
    );

    check(res, {
        'status is 200': (r) => r.status === 200,
        'has results array': (r) => {
            const body = JSON.parse(r.body);
            return Array.isArray(body.results);
        },
        'has summary': (r) => {
            const body = JSON.parse(r.body);
            return body.summary && body.summary.hasOwnProperty('success_count');
        },
        'has pagination info': (r) => {
            const body = JSON.parse(r.body);
            return body.page > 0 && body.total_users > 0;
        },
    });

    sleep(0.5);
}
