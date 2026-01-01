//go:build integration

package infra

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ✅ topology 只在 infra 声明
// tests 里只 Consume，不要重复 QueueDeclare/Bind（否则 406）
func EnsureRabbitTopology(ctx context.Context, rabbitURL string) error {
	_ = ctx // 这里留着以后你想加 ctx cancel/timeout 的 DialConfig 也方便

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	const exchange = "city.events"
	const queue = "it.auth.events"
	const binding = "auth.*"

	if err := ch.ExchangeDeclare(
		exchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	// ✅ 固定参数：durable=true, autoDelete=false, exclusive=false
	if _, err := ch.QueueDeclare(
		queue,
		true,  // durable
		false, // auto-delete ！！必须 false，避免 406
		false, // exclusive
		false,
		nil,
	); err != nil {
		return err
	}

	// FIX: Don't bind "auth.*" to this queue.
	// It prevents testing "NoRoute" scenarios because it catches everything.
	// Individual tests should declare their own bindings if they need to consume specific messages.
	// if err := ch.QueueBind(queue, binding, exchange, false, nil); err != nil {
	// 	return err
	// }

	return nil
}
