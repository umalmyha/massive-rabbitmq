package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/massive-rabbitmq/internal/broker"
	"github.com/umalmyha/massive-rabbitmq/internal/config"
	"github.com/umalmyha/massive-rabbitmq/internal/repository"
)

const (
	pgConnectTimeout   = 10 * time.Second
	rabbitConnAttempts = 5
)

func main() {
	cfg, err := config.Rabbit()
	if err != nil {
		logrus.Fatal(err)
	}

	pgConnStr, err := config.PostgresConnString()
	if err != nil {
		logrus.Fatal(err)
	}

	pgPool, err := postgresql(pgConnStr)
	if err != nil {
		logrus.Fatal(err)
	}
	defer pgPool.Close()

	rabbitConn, err := rabbitmq(cfg.ConnString)
	if err != nil {
		logrus.Fatal(err)
	}

	err = ensureTopology(rabbitConn, cfg)
	if err != nil {
		logrus.Fatal(err)
	}

	messageRps := repository.NewPostgresMessageRepository(pgPool)

	consumerCh, err := rabbitConn.Channel()
	if err != nil {
		logrus.Fatal(err)
	}

	consumer, err := broker.NewRabbitMQConsumer(consumerCh, messageRps)
	if err != nil {
		logrus.Fatal(err)
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt)

	go func() {
		if err := consumer.Consume(cfg.Queue); err != nil {
			logrus.Fatalf("error occurred while trying to start consumption - %v", err)
		}
	}()

	<-stopCh
	logrus.Infof("stopping consumer...")

	if err := consumerCh.Close(); err != nil {
		logrus.Errorf("failed to close consumer channel gracefully - %v", err)
	}

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

func ensureTopology(conn *amqp091.Connection, cfg *config.RabbitConfig) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel - %w", err)
	}

	err = ch.ExchangeDeclare(
		cfg.Exchange,
		amqp091.ExchangeDirect,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to define exchange - %w", err)
	}

	q, err := ch.QueueDeclare(
		cfg.Queue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to define queue - %w", err)
	}

	err = ch.QueueBind(q.Name, "", cfg.Exchange, false, nil)
	if err != nil {
		return err
	}

	if err := ch.Close(); err != nil {
		return fmt.Errorf("failed to close channel gracefully - %w", err)
	}

	return nil
}

func postgresql(uri string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pgConnectTimeout)
	defer cancel()

	pool, err := pgxpool.Connect(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to establish connection to db - %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("didn't get response from database after sending ping request - %w", err)
	}
	return pool, nil
}
