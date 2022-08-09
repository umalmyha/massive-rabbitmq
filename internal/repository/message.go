// Package repository contains repositories for entities
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/umalmyha/massive-rabbitmq/internal/model"
)

// MessageBatch represents behavior of batch for message entity
type MessageBatch interface {
	CreateMessage(*model.Message)
	Len() int
}

// MessageRepository represents behavior for message entity repository
type MessageRepository interface {
	NewBatch() MessageBatch
	SendBatch(context.Context, MessageBatch) error
}

type postgresMessageBatch struct {
	batch *pgx.Batch
}

// CreateMessage adds create Message request to batch
func (b *postgresMessageBatch) CreateMessage(m *model.Message) {
	q := "INSERT INTO messages(id, header, content) VALUES($1, $2, $3)"
	b.batch.Queue(q, m.ID, m.Header, m.Content)
}

// Len returns batch length
func (b *postgresMessageBatch) Len() int {
	return b.batch.Len()
}

type postgresMessageRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresMessageRepository builds new postgres repository for Message
func NewPostgresMessageRepository(p *pgxpool.Pool) MessageRepository {
	return &postgresMessageRepository{pool: p}
}

// NewBatch creates new batch for postgres
func (r *postgresMessageRepository) NewBatch() MessageBatch {
	return &postgresMessageBatch{batch: new(pgx.Batch)}
}

// SendBatch process batch passed in, batch must be postgres batch
func (r *postgresMessageRepository) SendBatch(ctx context.Context, batch MessageBatch) error {
	b, ok := batch.(*postgresMessageBatch)
	if !ok {
		return errors.New("provided batch is not postgres batch")
	}

	br := r.pool.SendBatch(ctx, b.batch)
	for i := 0; i < b.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed to perform batch request - %w", err)
		}
	}

	if err := br.Close(); err != nil {
		return fmt.Errorf("failed to close batch response connection - %w", err)
	}

	return nil
}
