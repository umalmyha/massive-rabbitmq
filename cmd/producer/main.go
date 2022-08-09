package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/massive-rabbitmq/internal/config"
	"github.com/umalmyha/massive-rabbitmq/internal/model"
	"github.com/umalmyha/massive-rabbitmq/internal/random"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	rabbitConnAttempts    = 5
	totalNumberOfMessages = 1_000_000
	sendBatchSize         = 2000
	msgHeaderLength       = 50
	msgContentLength      = 100
)

func main() {
	cfg, err := config.Rabbit()
	if err != nil {
		logrus.Fatal(err)
	}

	rabbitConn, err := rabbitmq(cfg.ConnString)
	if err != nil {
		logrus.Fatal(err)
	}

	publishMessages(rabbitConn, cfg.Exchange)

	if err := rabbitConn.Close(); err != nil {
		logrus.Fatal(err)
	}
}

func rabbitmq(uri string) (*amqp091.Connection, error) {
	backoff := time.Second

	for i := 0; i < rabbitConnAttempts; i++ {
		conn, err := amqp091.Dial(uri)
		if err == nil {
			return conn, nil
		}

		logrus.Infof("failed to connect to rabbitmq, try again after %s... - %v", backoff, err)
		<-time.After(backoff)
		backoff += time.Second
	}

	return nil, fmt.Errorf("failed to establish connection to rabbitmq, timeout %s exceeded", backoff)
}

func publishMessages(conn *amqp091.Connection, exchange string) {
	rabbitCh, err := conn.Channel()
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.Infof("starting to generate %d messages", totalNumberOfMessages)

	rabbitPubs := make([]amqp091.Publishing, totalNumberOfMessages)
	for i := 0; i < totalNumberOfMessages; i++ {
		m := &model.Message{
			ID:      uuid.NewString(),
			Header:  random.String(msgHeaderLength),
			Content: random.String(msgContentLength),
		}

		enc, err := msgpack.Marshal(m)
		if err != nil {
			logrus.Fatal(err)
		}

		rabbitPubs[i] = amqp091.Publishing{
			Headers:      amqp091.Table{},
			ContentType:  "application/x-msgpack",
			DeliveryMode: amqp091.Transient,
			Body:         enc,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt)

	go func() {
		ticker := time.NewTicker(time.Second)
		startPos := 0
		endPos := startPos + sendBatchSize

		for endPos <= totalNumberOfMessages {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for i := startPos; i < endPos; i++ {
					err := rabbitCh.PublishWithContext(ctx, exchange, "", true, false, rabbitPubs[i])
					if err != nil {
						logrus.Errorf("failed to deliver messages to rabbitmq - %v", err)
					}
				}
			}

			startPos += +sendBatchSize
			endPos = startPos + sendBatchSize

			logrus.Infof("%d messages were sent, %d left", startPos, totalNumberOfMessages-startPos)
		}

		stopCh <- os.Interrupt // finish if all messages are sent
	}()

	<-stopCh
	logrus.Info("stopping kafka rabbitmq publisher...")

	cancel()
	if err := rabbitCh.Close(); err != nil {
		logrus.Errorf("failed to close rabbitmq channel gracefully - %v", err)
	}
}
