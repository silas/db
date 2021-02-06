package sq

import (
	"context"
	"errors"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	ErrNoRows           = pgx.ErrNoRows
	ErrTxClosed         = pgx.ErrTxClosed
	ErrTxCommitRollback = pgx.ErrTxCommitRollback
)

type Config = pgxpool.Config

type Pool interface {
	Tx(ctx context.Context, fn func(tx Tx) error) error
	Close()
}

func Connect(ctx context.Context, connString string) (Pool, error) {
	config, err := ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	return ConnectConfig(ctx, config)
}

func ParseConfig(connString string) (*Config, error) {
	return pgxpool.ParseConfig(connString)
}

func ConnectConfig(ctx context.Context, config *Config) (Pool, error) {
	pool, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	return &pgxPool{pool: pool}, nil
}

type pgxPool struct {
	pool *pgxpool.Pool
}

func (p *pgxPool) Tx(ctx context.Context, fn func(tx Tx) error) error {
	pgxtx, err := p.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		return err
	}
	tx := &pgxTx{tx: pgxtx}
	return txExecute(ctx, tx, fn)
}

func (p *pgxPool) Close() {
	p.pool.Close()
}

type Result = pgconn.CommandTag

type Tx interface {
	Exec(ctx context.Context, qb StatementBuilder) (Result, error)
	Query(ctx context.Context, qb StatementBuilder) (Rows, error)
	QueryRow(ctx context.Context, qb StatementBuilder) Row
}

type pgxTx struct {
	tx pgx.Tx
}

func (tx *pgxTx) Exec(ctx context.Context, qb StatementBuilder) (Result, error) {
	sql, args, err := qb.ToSQL()
	if err != nil {
		return nil, err
	}

	sql, err = replacePlaceholders(sql)
	if err != nil {
		return nil, err
	}

	return tx.tx.Exec(ctx, sql, args...)
}

func (tx *pgxTx) Query(ctx context.Context, qb StatementBuilder) (Rows, error) {
	sql, args, err := qb.ToSQL()
	if err != nil {
		return nil, err
	}

	sql, err = replacePlaceholders(sql)
	if err != nil {
		return nil, err
	}

	return tx.tx.Query(ctx, sql, args...)
}

func (tx *pgxTx) QueryRow(ctx context.Context, qb StatementBuilder) Row {
	sql, args, err := qb.ToSQL()
	if err != nil {
		return rowError{err}
	}

	sql, err = replacePlaceholders(sql)
	if err != nil {
		return rowError{err}
	}

	return tx.tx.QueryRow(ctx, sql, args...)
}

type rowError struct {
	err error
}

func (e rowError) Scan(...interface{}) error {
	return e.err
}

func IsError(err error, code string) bool {
	if err == nil {
		return false
	}

	var e *pgconn.PgError
	return errors.As(err, &e) && e != nil && e.Code == code
}

type Row interface {
	Scan(...interface{}) error
}

type Rows interface {
	Row
	Next() bool
	Close()
}
