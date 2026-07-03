// Package postgres owns the `map` schema: the map_objects table with its PostGIS
// geography column and GiST index. It is the sole owner of that state; handlers
// go through it and never touch the driver directly. No cross-schema FKs live
// here — presence is Valkey-only and users/friends belong to other services.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested map object does not exist.
var ErrNotFound = errors.New("map object not found")

// Object is the persistent shape of a map object. Presence (visitor_count,
// friend_ids_here) is NOT stored here — it is computed from Valkey at read time.
type Object struct {
	ID         string
	ObjectType string // PARK | DOG_PARK | DOG_BEACH
	Name       string
	Longitude  float64
	Latitude   float64
	SourceOSM  int64 // 0 when unset
}

// Store owns the map schema via a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore opens a pgx pool against the given DSN and verifies connectivity.
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// ObjectsWithin returns every map object whose location is within radiusMeters
// of (lon, lat), using the PostGIS ST_DWithin over the geography column so the
// GiST index (map_objects_location_gix) can serve the query.
func (s *Store) ObjectsWithin(ctx context.Context, lon, lat float64, radiusMeters int) ([]Object, error) {
	const q = `
SELECT id::text,
       object_type,
       COALESCE(name, ''),
       ST_X(location::geometry) AS longitude,
       ST_Y(location::geometry) AS latitude,
       COALESCE(source_osm_id, 0)
FROM map.map_objects
WHERE ST_DWithin(location, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)`

	rows, err := s.pool.Query(ctx, q, lon, lat, radiusMeters)
	if err != nil {
		return nil, fmt.Errorf("query objects within: %w", err)
	}
	defer rows.Close()

	var out []Object
	for rows.Next() {
		var o Object
		if err := rows.Scan(&o.ID, &o.ObjectType, &o.Name, &o.Longitude, &o.Latitude, &o.SourceOSM); err != nil {
			return nil, fmt.Errorf("scan object: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ObjectByID returns a single map object, or ErrNotFound.
func (s *Store) ObjectByID(ctx context.Context, id string) (Object, error) {
	const q = `
SELECT id::text,
       object_type,
       COALESCE(name, ''),
       ST_X(location::geometry) AS longitude,
       ST_Y(location::geometry) AS latitude,
       COALESCE(source_osm_id, 0)
FROM map.map_objects
WHERE id = $1`

	var o Object
	err := s.pool.QueryRow(ctx, q, id).
		Scan(&o.ID, &o.ObjectType, &o.Name, &o.Longitude, &o.Latitude, &o.SourceOSM)
	if errors.Is(err, pgx.ErrNoRows) {
		return Object{}, ErrNotFound
	}
	if err != nil {
		return Object{}, fmt.Errorf("query object by id: %w", err)
	}
	return o, nil
}

// UpsertOSM inserts or updates a map object keyed by source_osm_id. Used by the
// OSM seeding job; map objects are read-only for MVP otherwise.
func (s *Store) UpsertOSM(ctx context.Context, objectType, name string, lon, lat float64, osmID int64) error {
	const q = `
INSERT INTO map.map_objects (object_type, name, location, source_osm_id)
VALUES ($1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography, $5)
ON CONFLICT (source_osm_id) DO UPDATE
SET object_type = EXCLUDED.object_type,
    name        = EXCLUDED.name,
    location    = EXCLUDED.location`

	_, err := s.pool.Exec(ctx, q, objectType, name, lon, lat, osmID)
	if err != nil {
		return fmt.Errorf("upsert osm object: %w", err)
	}
	return nil
}
