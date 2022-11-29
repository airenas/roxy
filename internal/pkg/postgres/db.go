package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/jackc/pgx/v5"
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

// InsertRequest inserts request into DB
func (db *DB) InsertRequest(ctx context.Context, req *persistence.ReqData) error {
	rows, err := db.pool.Query(ctx, `INSERT INTO requests(id, email, file_count, params, request_id, created) 
	VALUES($1, $2, $3, $4, $5, $6)`, req.ID, req.Email, req.FileCount,
		req.Params,
		req.RequestID,
		req.Created,
	)
	if err != nil {
		return fmt.Errorf("can't insert reguest: %w", err)
	}
	defer rows.Close()
	return nil
}

// LoadRequest loads request from DB
func (db *DB) LoadRequest(ctx context.Context, id string) (*persistence.ReqData, error) {
	var res persistence.ReqData
	err := db.pool.QueryRow(ctx, `SELECT id, email, file_count, params, request_id, created FROM requests
		WHERE id = $1`, id).Scan(&res.ID, &res.Email, &res.FileCount, &res.Params, &res.RequestID, &res.Created)
	if err != nil {
		return nil, fmt.Errorf("can't load reguest: %w", err)
	}
	return &res, nil
}

// LoadWorkData loads work info from DB
func (db *DB) LoadWorkData(ctx context.Context, id string) (*persistence.WorkData, error) {
	var res persistence.WorkData
	err := db.pool.QueryRow(ctx, `SELECT id, external_id, created FROM work_data
		WHERE id = $1`, id).Scan(&res.ID, &res.ExternalID, &res.Created)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("can't load reguest: %w", err)
	}
	return &res, nil
}

// InsertWorkData inserts data into DB
func (db *DB) InsertWorkData(ctx context.Context, data *persistence.WorkData) error {
	rows, err := db.pool.Query(ctx, `INSERT INTO work_data(id, external_id, created) 
	VALUES($1, $2, $3)`, data.ID, data.ExternalID, data.Created)
	if err != nil {
		return fmt.Errorf("can't insert work_data: %w", err)
	}
	defer rows.Close()
	return nil
}

// InsertStatus inserts status into DB
func (db *DB) InsertStatus(ctx context.Context, item *persistence.Status) error {
	rows, err := db.pool.Query(ctx, `INSERT INTO status(id, status, audio_ready, created) 
	VALUES($1, $2, $3, $4)`, item.ID, item.Status, item.AudioReady, item.Created)
	if err != nil {
		return fmt.Errorf("can't insert status: %w", err)
	}
	defer rows.Close()
	return nil
}

// LoadStatus loads work info from DB
func (db *DB) LoadStatus(ctx context.Context, id string) (*persistence.Status, error) {
	var res persistence.Status
	err := db.pool.QueryRow(ctx, `SELECT id, status, progress, error_code, error,
    audio_ready, available_results, version FROM status
		WHERE id = $1`, id).Scan(&res.ID, &res.Status, &res.Progress, &res.ErrorCode,
		&res.Error, &res.AudioReady, &res.AvailableResults, &res.Version)
	if err != nil {
		return nil, fmt.Errorf("can't load reguest: %w", err)
	}
	return &res, nil
}

// UpdateStatus updates status into DB
func (db *DB) UpdateStatus(ctx context.Context, item *persistence.Status) error {
	rows, err := db.pool.Exec(ctx, `UPDATE status SET 
	status = $3, 
	audio_ready = $4,
	available_results = $5,
	progress = $6, 
	error = $7, 
	error_code=$8,
	updated = $9,
	version = $2 + 1 
	WHERE id = $1 and version = $2`, item.ID, item.Version, item.Status,
		item.AudioReady, item.AvailableResults, item.Progress, item.Error, item.ErrorCode, time.Now())
	if err != nil {
		return fmt.Errorf("can't update status: %w", err)
	}
	if rows.RowsAffected() != 1 {
		return fmt.Errorf("can't update status, no records found")
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
