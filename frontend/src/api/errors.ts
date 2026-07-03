// A normalized error thrown by the apiClient. Callers (stores) can catch this
// and surface `message` via the toast layer. `code` is the backend's numeric
// code (Docs/02-Backend.md: "Every response carries code + message on error").
export class ApiError extends Error {
  readonly code: number
  readonly status: number

  constructor(message: string, code: number, status: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }

  /** True for auth/session failures that should force a re-login. */
  get isUnauthorized(): boolean {
    return this.status === 401
  }
}
