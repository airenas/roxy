package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBIdsProvider provides expired IDs from postgresql
type DBIdsProvider struct {
	pool         *pgxpool.Pool
	expiresAfter time.Duration
}

// NewDB creates Request instance
func NewDBIdsProvider(pool *pgxpool.Pool, expiresAfter time.Duration) (*DBIdsProvider, error) {
	res := &DBIdsProvider{pool: pool, expiresAfter: expiresAfter}
	return res, nil
}

// InsertRequest inserts request into DB
func (db *DBIdsProvider) GetExpired(ctx context.Context) ([]string, error) {
	exp := time.Now().Add(-db.expiresAfter)
	goapp.Log.Info().Time("older than", exp).Msg("selecting old records...")
	rows, err := db.pool.Query(ctx, `SELECT id FROM requests WHERE created < $1`, exp)
	if err != nil {
		return nil, fmt.Errorf("can't select IDs: %w", err)
	}
	defer rows.Close()

	res := []string{}
	for rows.Next() {
		var id string
		err := rows.Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("can't retrieve IDs: %w", err)
		}
		res = append(res, id)
	}

	return res, nil
}
