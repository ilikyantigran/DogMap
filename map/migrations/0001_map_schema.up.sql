-- Map service owns the `map` schema. One schema per service, NO cross-schema FKs
-- (map_objects references no users/friends; presence lives only in Valkey).
--
-- Requires the PostGIS extension for the geography type + spatial (GiST) index
-- that backs the 5km ST_DWithin radius query in LoadMap.

CREATE EXTENSION IF NOT EXISTS postgis;

CREATE SCHEMA IF NOT EXISTS map;

-- object_type is one of PARK | DOG_PARK | DOG_BEACH (enforced at the app layer to
-- keep parity with the proto enum; a CHECK guards against bad seed data).
CREATE TABLE IF NOT EXISTS map.map_objects (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type   text NOT NULL CHECK (object_type IN ('PARK', 'DOG_PARK', 'DOG_BEACH')),
    name          text,
    location      geography(Point, 4326) NOT NULL,
    source_osm_id bigint UNIQUE
);

-- Spatial index for the radius query: LoadMap runs
--   ST_DWithin(location, ST_MakePoint(lon, lat)::geography, 5000)
-- which the planner can satisfy with this GiST index on the geography column.
CREATE INDEX IF NOT EXISTS map_objects_location_gix
    ON map.map_objects
    USING GIST (location);
