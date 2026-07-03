// Command seed-osm is the OpenStreetMap seeding job for the Map service.
//
// STATUS: STUB for MVP. It lays out the real shape of the job — connect to the
// map schema, pull dog-relevant features from OSM (Overpass API or a regional
// extract), and upsert them keyed by source_osm_id — but the OSM fetch itself is
// left as a TODO. Map objects are READ-ONLY for MVP (no user edits); this job is
// the only writer.
//
// Query targets (Docs/02-Backend.md → "Map data seeding (OSM)"):
//   - leisure=park        -> PARK
//   - leisure=dog_park    -> DOG_PARK
//   - dog-friendly beaches -> DOG_BEACH
//
// Run: CONFIG_PATH=./configs/values_local.yaml go run ./cmd/seed-osm
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"map-service/internal/domain/postgres"
	"map-service/internal/infra/config"
)

// osmFeature is one seeded map object. object_type must be one of
// PARK | DOG_PARK | DOG_BEACH.
type osmFeature struct {
	ObjectType string
	Name       string
	Longitude  float64
	Latitude   float64
	SourceOSM  int64
}

func main() {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "./configs/values_local.yaml"
	}
	cfg, err := config.InitConfig(path)
	if err != nil {
		log.Fatalf("init config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := postgres.NewStore(ctx, cfg.Postgres.DSN)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer store.Close()

	features, err := fetchFeatures(ctx)
	if err != nil {
		log.Fatalf("fetch OSM features: %v", err)
	}

	for _, f := range features {
		if err := store.UpsertOSM(ctx, f.ObjectType, f.Name, f.Longitude, f.Latitude, f.SourceOSM); err != nil {
			log.Fatalf("upsert %s (osm %d): %v", f.Name, f.SourceOSM, err)
		}
	}
	log.Printf("seed-osm: upserted %d map objects", len(features))
}

// fetchFeatures pulls dog-relevant features from OSM.
//
// TODO(map): implement the real fetch. Query the Overpass API for
//
//	node/way/relation["leisure"="park"];
//	node/way/relation["leisure"="dog_park"];
//	node/way/relation["natural"="beach"]["dog"~"yes|leashed"];
//
// over a bounding box (or process a regional .osm.pbf extract), map each element
// to an osmFeature (centroid lon/lat for ways/relations, tag -> object_type),
// and return them. The upsert path above is already wired and idempotent, so
// wiring the fetch is the only remaining work.
func fetchFeatures(_ context.Context) ([]osmFeature, error) {
	// Stub: returns nothing so the job is a safe no-op until the fetch lands.
	return nil, nil
}
