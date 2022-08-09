// Package config contains application configuration logic
package config

import "github.com/caarlos0/env/v6"

// PostgresCfg is a configuration for postgres
type PostgresCfg struct {
	ConnString string `env:"POSTGRES_URL"`
}

// RabbitConfig is a configuration for rabbitmq
type RabbitConfig struct {
	ConnString string `env:"RABBITMQ_CONN_STRING"`
	Exchange   string `env:"RABBITMQ_EXCHANGE_NAME" envDefault:"messages-exchange"`
	Queue      string `env:"RABBITMQ_QUEUE_NAME" envDefault:"messages-queue"`
}

// Rabbit builds config for rabbitmq
func Rabbit() (*RabbitConfig, error) {
	var cfg RabbitConfig
	opts := env.Options{RequiredIfNoDef: true}

	if err := env.Parse(&cfg, opts); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// PostgresConnString returns postgres connection string
func PostgresConnString() (string, error) {
	var cfg PostgresCfg
	opts := env.Options{RequiredIfNoDef: true}

	if err := env.Parse(&cfg, opts); err != nil {
		return "", err
	}
	return cfg.ConnString, nil
}
