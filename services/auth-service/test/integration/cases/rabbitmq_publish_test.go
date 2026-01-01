//go:build integration

package cases

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	rmq "github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/messaging/rabbitmq"
	itinfra "github.com/baechuer/real-time-ressys/services/auth-service/test/integration/infra"
)

const (
	testExchange = "city.events" // 要和你 Publisher 的 DefaultExchange 对齐
)

// 这个测试要求你的 Publisher 启用 mandatory+confirm：
// - 没有任何 queue/binding 能路由到 => RabbitMQ Return => 你返回 error
func Test_RabbitMQ_NoRoute_ReturnsError(t *testing.T) {
	env, err := itinfra.LoadEnv()
	require.NoError(t, err)

	d := MustNewDeps(t, env)
	defer d.Close(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ch, err := d.AMQP.Channel()
	require.NoError(t, err)
	defer ch.Close()

	// 只保证 exchange 存在（不创建任何 queue/binding）
	require.NoError(t, ch.ExchangeDeclare(
		testExchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false,
		false,
		nil,
	))

	// FIX: rabbit_topology.go binds "auth.*" to "it.auth.events" by default.
	// We must unbind it to test "No Route" scenario.
	err = ch.QueueUnbind("it.auth.events", "auth.*", testExchange, nil)
	require.NoError(t, err, "failed to unbind queue")

	// DEBUG: Verify raw publish returns
	returnCh := ch.NotifyReturn(make(chan amqp091.Return, 1))
	confirmCh := ch.NotifyPublish(make(chan amqp091.Confirmation, 1))
	err = ch.PublishWithContext(ctx, testExchange, "auth.email.verify.requested", true, false, amqp091.Publishing{
		Body: []byte("{}"),
	})
	require.NoError(t, err)

	select {
	case <-confirmCh:
	case <-time.After(time.Second):
		t.Log("Timeout waiting for confirm")
	}

	select {
	case ret := <-returnCh:
		t.Logf("Raw Publish: Captured Return Code=%d Text=%s", ret.ReplyCode, ret.ReplyText)
	case <-time.After(2 * time.Second):
		t.Fatal("Raw Publish: Failed to capture Return! Binding still exists?")
	}

	pubImpl, ok := d.Pub.(*rmq.Publisher)
	require.True(t, ok, "d.Pub should be *rabbitmq.Publisher")

	ev := newVerifyEmailEventForIT(t)

	// 这里应该触发 no-route（mandatory Return）=> error
	err = pubImpl.PublishVerifyEmail(ctx, ev)
	require.Error(t, err)
}

func Test_RabbitMQ_Routed_Ack_AndMessageArrives(t *testing.T) {
	env, err := itinfra.LoadEnv()
	require.NoError(t, err)

	d := MustNewDeps(t, env)
	defer d.Close(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ch, err := d.AMQP.Channel()
	require.NoError(t, err)
	defer ch.Close()

	// 1) declare exchange (idempotent)
	require.NoError(t, ch.ExchangeDeclare(
		testExchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false,
		false,
		nil,
	))

	// 2) declare a temporary queue to avoid arg-conflicts
	//    server-named + exclusive + auto-delete = 不会和历史队列定义冲突
	q, err := ch.QueueDeclare(
		"",    // server generates name
		false, // durable
		true,  // auto-delete
		true,  // exclusive
		false, // no-wait
		nil,
	)
	require.NoError(t, err)

	// 3) bind wildcard to catch auth.* routing keys
	require.NoError(t, ch.QueueBind(
		q.Name,
		"auth.#", // 兜底：只要 Publisher routing key 在 auth.* 范围就能路由到
		testExchange,
		false,
		nil,
	))

	// 4) start consumer
	msgCh, err := ch.Consume(
		q.Name,
		"",    // consumer tag
		true,  // auto-ack
		true,  // exclusive (must match queue exclusive usage)
		false, // no-local
		false, // no-wait
		nil,
	)
	require.NoError(t, err)

	pubImpl, ok := d.Pub.(*rmq.Publisher)
	require.True(t, ok, "d.Pub should be *rabbitmq.Publisher")

	ev := newVerifyEmailEventForIT(t)

	// 5) publish
	require.NoError(t, pubImpl.PublishVerifyEmail(ctx, ev))

	// 6) assert message arrives
	select {
	case msg := <-msgCh:
		require.Greater(t, len(msg.Body), 0)

		// 尽量别强绑定 JSON 结构（你 event struct/serializer 可能会变）
		// 这里做一个弱校验：包含 email 字符串即可
		require.True(t, bytes.Contains(msg.Body, []byte("it@example.com")),
			"expected message body contains it@example.com, got=%s", string(msg.Body))

	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for message")
	}
}

// newVerifyEmailEventForIT:
// 你现在 VerifyEmailEvent 字段名不确定（Token/OTT/Link/...），所以用反射填充，保证编译不炸。
func newVerifyEmailEventForIT(t *testing.T) auth.VerifyEmailEvent {
	t.Helper()

	var ev auth.VerifyEmailEvent

	email := "it@example.com"
	token := "it-token-123"
	link := "https://frontend/verify-email?token=it-token-123"

	v := reflect.ValueOf(&ev).Elem()
	typ := v.Type()

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		sf := typ.Field(i)

		// 只处理可 set 的 string 字段
		if f.Kind() != reflect.String || !f.CanSet() {
			continue
		}

		name := strings.ToLower(sf.Name)

		switch {
		case strings.Contains(name, "email"):
			f.SetString(email)
		case strings.Contains(name, "token") || strings.Contains(name, "ott") || strings.Contains(name, "code"):
			f.SetString(token)
		case strings.Contains(name, "url") || strings.Contains(name, "link"):
			f.SetString(link)
		default:
			f.SetString("it")
		}
	}

	return ev
}

// （可选）如果你也想对 PasswordReset 做同样的路由测试，可以照抄上面的 Routed 测试，
// 把 newVerifyEmailEventForIT + PublishVerifyEmail 换成 PasswordResetEvent + PublishPasswordReset。
//
// 这里我没写是为了让你先把目前 failing 的这一个修到全绿。
var _ = amqp.ErrClosed
