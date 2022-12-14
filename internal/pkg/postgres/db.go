package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB provides operations with postgresql
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates Request instance
func NewDB(pool *pgxpool.Pool) (*DB, error) {
	res := &DB{pool: pool}
	return res, nil
}

// InsertRequest inserts request into DB
func (db *DB) InsertRequest(ctx context.Context, req *persistence.ReqData) error {
	rows, err := db.pool.Query(ctx, `INSERT INTO requests(id, email, file_count, file_name, file_names, params, request_id, created) 
	VALUES($1, $2, $3, $4, $5, $6, $7, $8)`, req.ID, req.Email, req.FileCount,
		req.FileName,
		req.FileNames,
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
	err := db.pool.QueryRow(ctx, `SELECT id, email, file_count, file_name, file_names, params, request_id, created FROM requests
		WHERE id = $1`, id).Scan(&res.ID, &res.Email, &res.FileCount, &res.FileName, &res.FileNames,
		&res.Params, &res.RequestID, &res.Created)
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

func (db *DB) DeleteWorkData(ctx context.Context, id string) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM work_data WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("can't delete work_data(%s): %w", id, err)
	}
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
	_, err := uuid.Parse(id) // otherwise psql complains on non uuid search
	if err != nil {
		return nil, nil
	}
	var res persistence.Status
	err = db.pool.QueryRow(ctx, `SELECT id, status, progress, error_code, error,
    audio_ready, available_results, recognized_text, version FROM status
		WHERE id = $1`, id).Scan(&res.ID, &res.Status, &res.Progress, &res.ErrorCode,
		&res.Error, &res.AudioReady, &res.AvailableResults, &res.RecognizedText, &res.Version)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
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
	recognized_text = $10,
	version = $2 + 1 
	WHERE id = $1 and version = $2`, item.ID, item.Version, item.Status,
		item.AudioReady, item.AvailableResults, item.Progress, item.Error, item.ErrorCode, time.Now(), item.RecognizedText)
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

const emailLockValue = 1

// LockEmailTable inserts record to db and marks it as working
func (db *DB) LockEmailTable(ctx context.Context, ID string, key string) error {
	goapp.Log.Info().Str("ID", ID).Str("key", key).Msg("locking")
	_, err := db.pool.Exec(ctx, `INSERT INTO email_lock(id, key, value) 
	VALUES($1, $2, $3) ON CONFLICT (id, key) DO NOTHING`, ID, key, 0)
	if err != nil {
		return fmt.Errorf("can't insert email_lock: %w", err)
	}

	res, err := db.pool.Exec(ctx,
		`UPDATE email_lock SET value = $4 WHERE id = $1 and key = $2 and value = $3`,
		ID, key, 0, emailLockValue)
	if err != nil {
		return fmt.Errorf("can't update email_lock: %w", err)
	}
	if res.RowsAffected() != 1 {
		return fmt.Errorf("can't lock email_lock, no records found")
	}
	goapp.Log.Info().Str("ID", ID).Str("key", key).Msg("locked")
	return nil
}

// UnLockEmailTable replaces table status with value
func (db *DB) UnLockEmailTable(ctx context.Context, ID string, key string, value int) error {
	goapp.Log.Info().Str("ID", ID).Str("key", key).Int("value", value).Msg("unlocking")
	res, err := db.pool.Exec(ctx,
		`UPDATE email_lock SET value = $4 WHERE id = $1 and key = $2 and value = $3`,
		ID, key, emailLockValue, value)
	if err != nil {
		return fmt.Errorf("can't unlock email_lock: %w", err)
	}
	if res.RowsAffected() != 1 {
		return fmt.Errorf("can't unlock email_lock, no records found")
	}
	goapp.Log.Info().Str("ID", ID).Str("key", key).Msg("unlocked")
	return nil
}
