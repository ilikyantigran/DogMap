import { describe, expect, it } from 'vitest'
import {
  isValidEmail,
  isValidPhone,
  passwordsMatch,
} from '@/lib/validation'

describe('validation', () => {
  it('validates email format', () => {
    expect(isValidEmail('a@b.com')).toBe(true)
    expect(isValidEmail('bad')).toBe(false)
    expect(isValidEmail('a@b')).toBe(false)
  })

  it('validates E.164 phone', () => {
    expect(isValidPhone('+14155552671')).toBe(true)
    expect(isValidPhone('4155552671')).toBe(false) // missing +
    expect(isValidPhone('+0155')).toBe(false) // country code cannot start with 0
  })

  it('checks password confirm match', () => {
    expect(passwordsMatch('secret', 'secret')).toBe(true)
    expect(passwordsMatch('secret', 'other')).toBe(false)
    expect(passwordsMatch('', '')).toBe(false)
  })
})
