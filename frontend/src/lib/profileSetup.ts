import type { UserInfo } from '@/types/api'

// Two-step registration: after a user's first confirmed login we send them to a
// one-time ProfileSetup step IF their profile is still empty AND they have not
// already skipped/completed it. The "already handled" flag is persisted to
// localStorage, keyed per user_id so it is per-account and survives refresh /
// re-login (mirrors authStore's localStorage ownership: storage access is
// centralized here so no other code touches these keys directly).

const SETUP_DONE_PREFIX = 'dogmap.setupDone.'

function key(userId: string): string {
  return `${SETUP_DONE_PREFIX}${userId}`
}

/** True once the user has completed OR skipped the setup step (persistent). */
export function isSetupDone(userId: string): boolean {
  return localStorage.getItem(key(userId)) === '1'
}

/** Mark the setup step handled (completed or skipped) so it never nags again. */
export function markSetupDone(userId: string): void {
  localStorage.setItem(key(userId), '1')
}

/**
 * A profile counts as "empty" (needs setup) when BOTH name and surname are
 * blank. Phone and pets are intentionally not counted — per the product spec the
 * setup trigger is name+surname only.
 */
export function isProfileEmpty(profile: Pick<UserInfo, 'name' | 'surname'>): boolean {
  return profile.name.trim() === '' && profile.surname.trim() === ''
}
