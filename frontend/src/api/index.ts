import { createApiClient, type ApiClient } from './apiClient'

// The app-wide apiClient singleton. It is wired at bootstrap (src/main.ts) with
// callbacks that read the current token and react to 401s. We inject them rather
// than importing the auth store here to avoid an import cycle
// (store -> api -> store) and to keep the client independently testable.

let getTokenFn: () => string | null = () => null
let onUnauthorizedFn: () => void = () => {}

// Empty base by default: stores call absolute gateway paths ('/v1/...') and the
// Vite dev proxy (see vite.config.ts) routes each service prefix to its port.
// Override with VITE_API_BASE_URL when a single aggregating gateway exists.
export const apiBaseUrl: string = import.meta.env.VITE_API_BASE_URL ?? ''

export const api: ApiClient = createApiClient({
  baseUrl: apiBaseUrl,
  getToken: () => getTokenFn(),
  onUnauthorized: () => onUnauthorizedFn(),
})

/** Wire the client to the running app. Call once, after Pinia is installed. */
export function configureApi(hooks: {
  getToken: () => string | null
  onUnauthorized: () => void
}): void {
  getTokenFn = hooks.getToken
  onUnauthorizedFn = hooks.onUnauthorized
}
