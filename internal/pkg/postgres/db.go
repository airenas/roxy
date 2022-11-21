package postgres

import (
	"context"
	"fmt"

	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB provides operations with postgresql
type DB struct {
	pool *pgxpool.Pool
}

//NewDB creates Request instance
func NewDB(pool *pgxpool.Pool) (*DB, error) {
	res := &DB{pool: pool}
	return res, nil
}

// SaveRequest inserts request into DB
func (db *DB) SaveRequest(ctx context.Context, req *persistence.ReqData) error {
	_, err := db.pool.Query(ctx, `INSERT INTO requests(id, email, file_count, params, request_id, created) 
	VALUES($1, $2, $3, $4, $5, $6)`, req.ID, req.Email, req.FileCount,
		req.Params,
		req.RequestID,
		req.Created,
	)
	if err != nil {
		return fmt.Errorf("can't insert reguest: %w", err)
	}
	return nil
}

// SaveStatus inserts status into DB
func (db *DB) SaveStatus(ctx context.Context, item *persistence.Status) error {
	_, err := db.pool.Query(ctx, `INSERT INTO status(id, status, audio_ready, created) 
	VALUES($1, $2, $3, $4)`, item.ID, item.Status, item.AudioReady, item.Created)
	if err != nil {
		return fmt.Errorf("can't insert status: %w", err)
	}
	return nil
}

// Live returns no error if db is reachable and initialized
func (db *DB) Live(ctx context.Context) error {
	var exists bool
	if err := db.pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM pg_tables WHERE tablename = 'gue_jobs')`).Scan(&exists); err != nil {
		return fmt.Errorf("can't check table: %w", err)
	}
	if !exists {
		return fmt.Errorf("no migration done")
	}
	return nil
}
