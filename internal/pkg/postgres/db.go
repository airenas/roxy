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

// SaveRequest implements upload.RequestSaver
func (db *DB) SaveRequest(ctx context.Context, req *persistence.ReqData) error {
	_, err := db.pool.Query(ctx, `INSERT INTO requests(id, email, file_count, params, audio_ready, request_id, created) 
	VALUES($1, $2, $3, $4, $5, $6, $7)`, req.ID, req.Email, req.FileCount,
		req.Params,
		req.AudioReady,
		req.RequestID,
		req.Created,
	)
	if err != nil {
		return fmt.Errorf("can't insert: %w", err)
	}
	return nil
}
