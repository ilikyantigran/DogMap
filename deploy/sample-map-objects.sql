-- Sample map objects for trying the app before the real OSM seeder is implemented.
-- Placed within ~5km of the frontend's DEFAULT_CENTER (London, 51.5074, -0.1278),
-- so they show up on the Map page when you DENY the browser geolocation prompt
-- (denying falls back to that default center).
--
-- Run against the composed Postgres:
--   docker compose exec -T postgres psql -U postgres -d dogmap < deploy/sample-map-objects.sql

INSERT INTO map.map_objects (object_type, name, location) VALUES
  ('DOG_PARK',  'Hyde Park Dog Area',   ST_SetSRID(ST_MakePoint(-0.1637, 51.5073), 4326)::geography),
  ('PARK',      'Green Park',           ST_SetSRID(ST_MakePoint(-0.1426, 51.5041), 4326)::geography),
  ('PARK',      'St James''s Park',     ST_SetSRID(ST_MakePoint(-0.1340, 51.5024), 4326)::geography),
  ('DOG_BEACH', 'Thames Foreshore Spot',ST_SetSRID(ST_MakePoint(-0.1195, 51.5085), 4326)::geography)
ON CONFLICT DO NOTHING;
