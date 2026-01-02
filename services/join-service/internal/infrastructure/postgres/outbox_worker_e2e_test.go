//go:build integration
// +build integration

package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func rabbitURLCandidates() []string {
	// 优先尝试你脚本可能会设置的变量名（不确定就多兼容几个）
	envs := []string{
		"TEST_RABBIT_URL",
		"RABBIT_URL",
		"RABBITMQ_URL",
	}

	var cands []string
	for _, k := range envs {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			cands = append(cands, v)

			// 如果误给了 15672（management），强制补一个 5672 版本
			if u, err := url.Parse(v); err == nil && u.Port() == "15672" {
				u.Host = strings.Replace(u.Host, ":15672", ":5672", 1)
				cands = append(cands, u.String())
			}
		}
	}

	// 强兜底（Windows / Docker Desktop 常用）
	cands = append(cands,
		"amqp://guest:guest@127.0.0.1:5672/",
		"amqp://guest:guest@localhost:5672/",
		"amqp://guest:guest@127.0.0.1:5673/",
		"amqp://guest:guest@localhost:5673/",
	)
	return cands
}

func dialRabbit(t *testing.T) (*amqp.Connection, string) {
	t.Helper()

	var last error
	for _, u := range rabbitURLCandidates() {
		conn, err := amqp.Dial(u)
		if err == nil {
			return conn, u
		}
		last = err
	}
	require.NoError(t, last, "unable to dial rabbitmq with any candidate url")
	return nil, ""
}

func declareExchangeQueue(t *testing.T, conn *amqp.Connection, exchange string, bindKey string) (*amqp.Channel, amqp.Queue) {
	t.Helper()

	ch, err := conn.Channel()
	require.NoError(t, err)

	// topic exchange
	require.NoError(t, ch.ExchangeDeclare(
		exchange,
		"topic",
		true,  // durable
		false, // autoDelete
		false,
		false,
		nil,
	))

	q, err := ch.QueueDeclare(
		"",    // random name
		false, // durable
		true,  // autoDelete
		true,  // exclusive
		false,
		nil,
	)
	require.NoError(t, err)

	require.NoError(t, ch.QueueBind(q.Name, bindKey, exchange, false, nil))
	return ch, q
}

func readOutboxRow(ctx context.Context, pool *pgxpool.Pool, traceID string, routingKey string) (status string, attempt int, nextRetry *time.Time, lastErr *string, err error) {
	row := pool.QueryRow(ctx,
		`SELECT status::text, attempt, next_retry_at, last_error
		   FROM outbox
		  WHERE trace_id=$1 AND routing_key=$2
		  ORDER BY occurred_at DESC
		  LIMIT 1`,
		traceID, routingKey,
	)
	err = row.Scan(&status, &attempt, &nextRetry, &lastErr)
	return
}

func waitUntil(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestOutboxWorker_E2E_PublishesAndMarksSent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	repo, pool := setupRepo(t)

	conn, rabbitURL := dialRabbit(t)
	defer conn.Close()

	const exchange = "city.events"
	ch, q := declareExchangeQueue(t, conn, exchange, "join.*")
	defer ch.Close()

	eventID := uuid.New()
	userID := uuid.New()
	traceID := "trace-outbox-ok"

	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))

	// 触发 outbox：JoinEvent 会插入 join.created
	_, err := repo.JoinEvent(ctx, traceID, "", eventID, userID)
	require.NoError(t, err)

	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	repo.StartOutboxWorker(workerCtx, rabbitURL, exchange)

	msgs, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	require.NoError(t, err)

	select {
	case <-msgs:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatalf("did not receive message on queue %s", q.Name)
	}

	// outbox 应该从 pending -> sent
	waitUntil(t, 3*time.Second, func() bool {
		status, _, _, _, err := readOutboxRow(ctx, pool, traceID, "join.created")
		return err == nil && status == "sent"
	})
}

func TestOutboxWorker_MandatoryNoRoute_IncrementsAttemptAndSchedulesRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	repo, pool := setupRepo(t)

	conn, rabbitURL := dialRabbit(t)
	defer conn.Close()

	// 用一个全新的、没有任何绑定的 exchange，确保触发 NO_ROUTE
	const exchange = "noroute.exchange"
	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	require.NoError(t, ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil))

	eventID := uuid.New()
	userID := uuid.New()
	traceID := "trace-outbox-noroute"

	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))
	_, err = repo.JoinEvent(ctx, traceID, "", eventID, userID)
	require.NoError(t, err)

	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	// 关键：用 dial 成功的 rabbitURL，不要用可能错误的 env
	repo.StartOutboxWorker(workerCtx, rabbitURL, exchange)

	// 等 worker 扫描 1-2 次（你默认 1s ticker）
	time.Sleep(3 * time.Second)

	status, attempt, nextRetry, lastErr, err := readOutboxRow(ctx, pool, traceID, "join.created")
	require.NoError(t, err)

	// 你的实现可能：仍 pending，但 attempt+1 且 next_retry_at 未来
	require.True(t, status == "pending" || status == "failed", "unexpected status: %s", status)
	require.GreaterOrEqual(t, attempt, 1)
	require.NotNil(t, nextRetry)
	require.True(t, nextRetry.After(time.Now().Add(-1*time.Second)))
	if lastErr != nil {
		// 不强依赖具体字符串，但至少应该有错误信息
		require.NotEmpty(t, *lastErr)
	}
}

func TestOutboxWorker_Idempotent_NoDoubleSendAfterSent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	repo, pool := setupRepo(t)

	conn, rabbitURL := dialRabbit(t)
	defer conn.Close()

	const exchange = "city.events"
	ch, q := declareExchangeQueue(t, conn, exchange, "join.*")
	defer ch.Close()

	eventID := uuid.New()
	userID := uuid.New()
	traceID := "trace-outbox-idem"

	require.NoError(t, repo.InitCapacity(ctx, eventID, 1))
	_, err := repo.JoinEvent(ctx, traceID, "", eventID, userID)
	require.NoError(t, err)

	msgs, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	require.NoError(t, err)

	// run #1
	workerCtx1, cancel1 := context.WithCancel(ctx)
	repo.StartOutboxWorker(workerCtx1, rabbitURL, exchange)

	select {
	case <-msgs:
	case <-time.After(3 * time.Second):
		t.Fatalf("did not receive first message")
	}

	waitUntil(t, 3*time.Second, func() bool {
		status, _, _, _, err := readOutboxRow(ctx, pool, traceID, "join.created")
		return err == nil && status == "sent"
	})
	cancel1()

	// 清空消息后，run #2 不应再发
	workerCtx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	repo.StartOutboxWorker(workerCtx2, rabbitURL, exchange)

	time.Sleep(1200 * time.Millisecond)

	ins, err := ch.QueueInspect(q.Name)
	require.NoError(t, err)
	require.Equal(t, 0, ins.Messages, "should not publish again after outbox is sent")

	_ = errors.New // keep imports stable if you remove something
}
