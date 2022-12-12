package postgres

import (
	"context"
	"fmt"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cleaner cleans all records related with ID
type Cleaner struct {
	pool   *pgxpool.Pool
	tables []string
}

// NewDB creates Request instance
func NewCleaner(pool *pgxpool.Pool) (*Cleaner, error) {
	res := &Cleaner{pool: pool, tables: []string{"status", "work_data", "email_lock", "requests"}}
	return res, nil
}

// InsertRequest inserts request into DB
func (db *Cleaner) Clean(ctx context.Context, id string) error {
	for _, t := range db.tables {
		cmd, err := db.pool.Exec(ctx, `DELETE FROM `+t+` WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("can't delete %s(%s): %w", id, t, err)
		}
		goapp.Log.Info().Str("ID", id).Str("table", t).Int64("rows", cmd.RowsAffected()).Msg("deleted")
	}
	return nil
}
