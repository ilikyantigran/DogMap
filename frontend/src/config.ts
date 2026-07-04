// Tunable client-side constants. Kept in one place so the polling cadence is
// obvious and matches the backend TTL contract (Docs/02-Backend.md).

// Presence TTL on the backend is 15 min. The heartbeat MUST be well under that
// so presence never expires mid-walk. Docs specify every 2-3 min; we use 2 min.
export const PRESENCE_HEARTBEAT_MS = 2 * 60 * 1000

// Map refresh cadence to update visitor_count / friends_here. Kept fairly tight
// so counts stay fresh; also refreshed on demand (object click + visiting toggle).
export const MAP_REFRESH_MS = 12 * 1000

// Profile/friends refresh for request + on-walk status updates.
export const FRIENDS_REFRESH_MS = 30 * 1000

// Default map center used before geolocation resolves (fallback: manual pan).
// London — arbitrary; only affects the very first LoadMap before geolocation.
export const DEFAULT_CENTER = { latitude: 51.5074, longitude: -0.1278 }
export const DEFAULT_ZOOM = 14
