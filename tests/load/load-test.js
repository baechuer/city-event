import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const feedLatency = new Trend('feed_latency');
const eventDetailLatency = new Trend('event_detail_latency');

// Configuration
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
    scenarios: {
        // Scenario 1: Smoke test (sanity check)
        smoke: {
            executor: 'constant-vus',
            vus: 1,
            duration: '10s',
            startTime: '0s',
        },
        // Scenario 2: Load test (normal load)
        load: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '30s', target: 10 },  // Ramp up to 10 users
                { duration: '1m', target: 10 },   // Stay at 10 users
                { duration: '30s', target: 50 },  // Ramp up to 50 users
                { duration: '1m', target: 50 },   // Stay at 50 users
                { duration: '30s', target: 0 },   // Ramp down
            ],
            startTime: '15s',
        },
        // Scenario 3: Stress test (find breaking point)
        stress: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '1m', target: 100 },  // Ramp to 100 users
                { duration: '2m', target: 100 },  // Hold at 100
                { duration: '1m', target: 200 },  // Ramp to 200 users
                { duration: '2m', target: 200 },  // Hold at 200
                { duration: '1m', target: 0 },    // Ramp down
            ],
            startTime: '5m', // Start after load test
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<500'],  // 95% of requests should be < 500ms
        errors: ['rate<0.1'],               // Error rate < 10%
    },
};

// Test functions
export default function () {
    // Test 1: Feed endpoint (Anonymous)
    const feedResponse = http.get(`${BASE_URL}/api/feed?type=trending&limit=10`);

    check(feedResponse, {
        'feed status 200': (r) => r.status === 200,
        'feed has items': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.items !== undefined;
            } catch {
                return false;
            }
        },
    }) || errorRate.add(1);

    feedLatency.add(feedResponse.timings.duration);

    sleep(0.5);

    // Test 2: Events list
    const eventsResponse = http.get(`${BASE_URL}/api/events?limit=10`);

    check(eventsResponse, {
        'events status 200': (r) => r.status === 200,
    }) || errorRate.add(1);

    sleep(0.5);

    // Test 3: Event detail (if we have events)
    try {
        const eventsBody = JSON.parse(eventsResponse.body);
        if (eventsBody.items && eventsBody.items.length > 0) {
            const eventId = eventsBody.items[0].id;
            const detailResponse = http.get(`${BASE_URL}/api/events/${eventId}/view`);

            check(detailResponse, {
                'event detail status 200': (r) => r.status === 200,
            }) || errorRate.add(1);

            eventDetailLatency.add(detailResponse.timings.duration);
        }
    } catch {
        // Events might be empty
    }

    sleep(1);
}

// Lifecycle hooks
export function setup() {
    console.log(`Starting load test against ${BASE_URL}`);

    // Health check
    const health = http.get(`${BASE_URL}/api/healthz`);
    if (health.status !== 200) {
        throw new Error(`Health check failed: ${health.status}`);
    }

    return { startTime: new Date().toISOString() };
}

export function teardown(data) {
    console.log(`Load test completed. Started at: ${data.startTime}`);
}

// Summary handler
export function handleSummary(data) {
    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }),
        'load-test-results.json': JSON.stringify(data, null, 2),
    };
}

function textSummary(data, options) {
    const metrics = data.metrics;

    let summary = `
=================================================================
                    LOAD TEST SUMMARY
=================================================================

📊 Request Metrics:
   Total Requests:     ${metrics.http_reqs?.values?.count || 0}
   Request Rate:       ${metrics.http_reqs?.values?.rate?.toFixed(2) || 0} req/s
   
⏱️ Latency (p95):
   All Requests:       ${metrics.http_req_duration?.values?.['p(95)']?.toFixed(2) || 0}ms
   Feed Endpoint:      ${metrics.feed_latency?.values?.['p(95)']?.toFixed(2) || 0}ms
   Event Detail:       ${metrics.event_detail_latency?.values?.['p(95)']?.toFixed(2) || 0}ms

❌ Errors:
   Error Rate:         ${(metrics.errors?.values?.rate * 100 || 0).toFixed(2)}%
   Failed Requests:    ${metrics.http_req_failed?.values?.passes || 0}

✅ Thresholds:
   http_req_duration p(95)<500ms: ${data.thresholds?.http_req_duration?.ok ? 'PASS' : 'FAIL'}
   errors rate<0.1:               ${data.thresholds?.errors?.ok ? 'PASS' : 'FAIL'}

=================================================================
`;
    return summary;
}
