import { ApiError } from '@/api/errors'
import { useToastStore } from '@/stores/toastStore'

// Small helper so components can surface a failed store action as a toast in one
// line. The 401 case is already handled centrally by the apiClient (session
// cleared + redirect), so we don't double-report it here.
export function toastError(err: unknown): void {
  const toast = useToastStore()
  if (err instanceof ApiError) {
    if (err.isUnauthorized) return
    toast.error(err.message)
    return
  }
  toast.error(err instanceof Error ? err.message : 'Something went wrong')
}
