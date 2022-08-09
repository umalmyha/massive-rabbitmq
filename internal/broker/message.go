// Package broker contains logic for message broker consumption
package broker

import (
	"context"
	"fmt"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/massive-rabbitmq/internal/model"
	"github.com/umalmyha/massive-rabbitmq/internal/repository"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	datasourceWriteTimeout = 5 * time.Second
	batchSize              = 2000
	consumerName           = "messages-consumer"
)

// RabbitMQConsumer is a consumer of rabbitmq queue
type RabbitMQConsumer struct {
	channel    *amqp091.Channel
	messageRps repository.MessageRepository
}

// NewRabbitMQConsumer builds rabbitmq consumer
func NewRabbitMQConsumer(ch *amqp091.Channel, messageRps repository.MessageRepository) (*RabbitMQConsumer, error) {
	if err := ch.Qos(batchSize, 0, false); err != nil {
		return nil, fmt.Errorf("failed to set QoS - %w", err)
	}
	return &RabbitMQConsumer{channel: ch, messageRps: messageRps}, nil
}

// Consume starts consumption from queue
func (c *RabbitMQConsumer) Consume(queue string) error {
	totalMessagesSaved := 0
	batch := c.messageRps.NewBatch()

	logrus.Infof("starting consumption...")
	msgs, err := c.channel.Consume(queue, consumerName, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to start consumption - %w", err)
	}

	for m := range msgs {
		var msg model.Message
		if err := msgpack.Unmarshal(m.Body, &msg); err != nil {
			logrus.Errorf("failed to deserize rabbitmq message %s - %v", m.MessageId, err)
			continue
		}

		batch.CreateMessage(&msg)

		batchLen := batch.Len()
		if batchLen < batchSize {
			continue
		}

		logrus.Infof("batch size %d reached - saving to datasource", batchLen)
		if err := c.sendBatch(batch); err != nil {
			logrus.Errorf("failed to save last %d messages - %v", batchLen, err)
		}

		totalMessagesSaved += batchLen
		logrus.Infof("total messages saved %d", totalMessagesSaved)

		batch = c.messageRps.NewBatch()

		if err := m.Ack(true); err != nil {
			logrus.Errorf("failed to ack last %d messages - %v", batchLen, err)
		}
	}

	// consumption stopped, but some messages can await save
	if batch.Len() > 0 {
		if err := c.sendBatch(batch); err != nil {
			logrus.Errorf("failed to save last %d messages - %v", batch.Len(), err)
		}
	}

	return nil
}

func (c *RabbitMQConsumer) sendBatch(b repository.MessageBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), datasourceWriteTimeout)
	defer cancel()

	return c.messageRps.SendBatch(ctx, b)
}
