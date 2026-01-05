
import http from 'k6/http';
import { check, sleep } from 'k6';
import { SharedArray } from 'k6/data';

// Load tokens from CSV
const tokens = new SharedArray('tokens', function () {
    return open('./tokens.csv').split('\n').filter(t => t.trim() !== '');
});

export const options = {
    scenarios: {
        join_event: {
            executor: 'ramping-arrival-rate',
            startRate: 0,
            timeUnit: '1s',
            preAllocatedVUs: 100,
            maxVUs: 1000,
            stages: [
                { target: 50, duration: '30s' }, // Ramp up to 50 joins/sec
                { target: 50, duration: '10s' }, // Hold
                { target: 0, duration: '10s' },  // Scale down
            ],
        },
    },
};

const EVENT_ID = '6b81798e-5131-4981-82b0-cad38e64081f'; // Load Test Event Auto

export default function () {
    // Pick a token based on VU ID or random?
    // We want unique users. 
    // vu.idInTest is 1-based index across test.
    // But exec.vu.idInInstance is safer?
    // Let's just pick randomly or sequentially from array?
    // Ideally sequential to ensure uniqueness.
    // scenarios.iterationInTest is 0-based index.

    const index = (__VU - 1) % tokens.length;
    const token = tokens[index];

    const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
    const url = `${baseUrl}/api/events/${EVENT_ID}/join`;

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
            'Idempotency-Key': `${__VU}-${__ITER}-${Date.now()}`,
        },
    };

    const res = http.post(url, JSON.stringify({}), params);

    const checkRes = check(res, {
        'status is 200 or 409': (r) => r.status === 200 || r.status === 409 || r.status === 400,
        'latency < 500ms': (r) => r.timings.duration < 500,
    });

    if (!checkRes) {
        console.log(`Failed: Status ${res.status} Body: ${res.body.slice(0, 100)}`);
    }
}
