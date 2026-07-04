// Vitest global setup. Kept minimal; individual specs mock what they need.
// happy-dom provides a DOM. Reset localStorage between tests so the auth store's
// persisted session doesn't leak from one spec into the next.
import { beforeEach } from 'vitest'

beforeEach(() => {
  localStorage.clear()
})
