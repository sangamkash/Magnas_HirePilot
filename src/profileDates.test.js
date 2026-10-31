import test from 'node:test'
import assert from 'node:assert/strict'
import { fromISODate, normalizeProfileDates, toISODate } from './profileDates.js'

test('converts valid day/month/year and month-only profile dates to ISO dates', () => {
  assert.equal(toISODate('31/10/2026'), '2026-10-31')
  assert.equal(toISODate('2026-10'), '2026-10-01')
  assert.equal(toISODate('2024-02-29'), '2024-02-29')
})

test('rejects impossible or malformed profile dates', () => {
  for (const value of ['31/02/2026', '32/01/2026', '2026-13', 'not a date']) {
    assert.equal(toISODate(value), '', `${value} should be rejected`)
  }
})

test('converts the date picker value for the visible date field', () => {
  assert.equal(fromISODate('2026-10-31'), '31/10/2026')
  assert.equal(fromISODate(''), '')
})

test('normalizes dates in every profile section sent to the API', () => {
  const input = {
    experience: [{ startDate: '01/06/2024', endDate: '', current: true }],
    education: [{ startDate: '2020-09', endDate: '2024-05' }],
    certificates: [{ issueDate: '15/04/2025', expirationDate: '' }],
    awards: [{ date: '2023-11' }],
  }
  assert.deepEqual(normalizeProfileDates(input), {
    ...input,
    experience: [{ startDate: '2024-06-01', endDate: '', current: true }],
    education: [{ startDate: '2020-09-01', endDate: '2024-05-01' }],
    certificates: [{ issueDate: '2025-04-15', expirationDate: '' }],
    awards: [{ date: '2023-11-01' }],
  })
  assert.equal(input.experience[0].startDate, '01/06/2024', 'normalization should not mutate editor state')
})
