import { afterEach, describe, expect, it } from 'vitest'
import {
  isProfileEmpty,
  isSetupDone,
  markSetupDone,
} from '@/lib/profileSetup'

afterEach(() => {
  localStorage.clear()
})

describe('profileSetup flag', () => {
  it('is not done for a fresh user', () => {
    expect(isSetupDone('u1')).toBe(false)
  })

  it('is done after markSetupDone', () => {
    markSetupDone('u1')
    expect(isSetupDone('u1')).toBe(true)
  })

  it('is keyed per user (does not leak across accounts)', () => {
    markSetupDone('u1')
    expect(isSetupDone('u1')).toBe(true)
    expect(isSetupDone('u2')).toBe(false)
  })

  it('survives a fresh read (persisted to localStorage)', () => {
    markSetupDone('u1')
    // Simulate a reload: read again with no in-memory state.
    expect(isSetupDone('u1')).toBe(true)
  })
})

describe('isProfileEmpty', () => {
  it('is empty when name and surname are both blank', () => {
    expect(isProfileEmpty({ name: '', surname: '' })).toBe(true)
  })

  it('treats whitespace-only name/surname as empty', () => {
    expect(isProfileEmpty({ name: '  ', surname: '\t' })).toBe(true)
  })

  it('is not empty when name is set', () => {
    expect(isProfileEmpty({ name: 'Ada', surname: '' })).toBe(false)
  })

  it('is not empty when surname is set', () => {
    expect(isProfileEmpty({ name: '', surname: 'Lovelace' })).toBe(false)
  })
})
