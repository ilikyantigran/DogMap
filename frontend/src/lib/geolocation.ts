// Wrap the browser Geolocation API in a promise. Used only to CENTER the map for
// LoadMap — we never stream coordinates (presence is point-of-interest, not GPS;
// see Docs/01-Idea.md). Fallback on denial/error is the default center + manual pan.

export interface Coords {
  latitude: number
  longitude: number
}

export function getCurrentPosition(timeoutMs = 8000): Promise<Coords> {
  return new Promise((resolve, reject) => {
    if (typeof navigator === 'undefined' || !navigator.geolocation) {
      reject(new Error('Geolocation is not available'))
      return
    }
    navigator.geolocation.getCurrentPosition(
      (pos) =>
        resolve({
          latitude: pos.coords.latitude,
          longitude: pos.coords.longitude,
        }),
      (err) => reject(new Error(err.message || 'Geolocation failed')),
      { timeout: timeoutMs, enableHighAccuracy: false },
    )
  })
}
