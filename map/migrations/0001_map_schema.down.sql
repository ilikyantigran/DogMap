DROP INDEX IF EXISTS map.map_objects_location_gix;
DROP TABLE IF EXISTS map.map_objects;
DROP SCHEMA IF EXISTS map;
-- The postgis extension is intentionally NOT dropped: other schemas may use it.
