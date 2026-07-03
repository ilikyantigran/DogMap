import { defineStore } from 'pinia'

// Centralized toast/notification surface. Stores push messages here (typically
// the backend `code`/`message` from an ApiError) instead of components owning
// error UI. Cross-cutting concern from Docs/03-Frontend.md.

export type ToastKind = 'error' | 'success' | 'info'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

interface ToastState {
  toasts: Toast[]
  nextId: number
}

const AUTO_DISMISS_MS = 5000

export const useToastStore = defineStore('toast', {
  state: (): ToastState => ({
    toasts: [],
    nextId: 1,
  }),
  actions: {
    push(message: string, kind: ToastKind = 'info') {
      const id = this.nextId++
      this.toasts.push({ id, kind, message })
      // Auto-dismiss. Guarded for non-browser (test) environments.
      if (typeof window !== 'undefined') {
        window.setTimeout(() => this.dismiss(id), AUTO_DISMISS_MS)
      }
      return id
    },
    error(message: string) {
      return this.push(message, 'error')
    },
    success(message: string) {
      return this.push(message, 'success')
    },
    info(message: string) {
      return this.push(message, 'info')
    },
    dismiss(id: number) {
      this.toasts = this.toasts.filter((t) => t.id !== id)
    },
  },
})
