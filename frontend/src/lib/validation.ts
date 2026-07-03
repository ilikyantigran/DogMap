// Client-side validation helpers. These are UX guards only — the backend is the
// real authority. Covers the checks named in Docs/03-Frontend.md: email format,
// password confirm match, phone E.164.

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
// E.164: leading +, country code (1-9) then up to 14 more digits.
const E164_RE = /^\+[1-9]\d{1,14}$/

export function isValidEmail(email: string): boolean {
  return EMAIL_RE.test(email.trim())
}

export function isValidPhone(phone: string): boolean {
  return E164_RE.test(phone.trim())
}

export function passwordsMatch(a: string, b: string): boolean {
  return a.length > 0 && a === b
}

export function isNonEmpty(v: string): boolean {
  return v.trim().length > 0
}
