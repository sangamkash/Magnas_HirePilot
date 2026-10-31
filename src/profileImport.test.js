import test from 'node:test'
import assert from 'node:assert/strict'
import { importLegacyProfiles, stableJSON } from './profileImport.js'

const profile = index => ({
  id: `old-${index}`,
  legacyImportId: `legacy-${index}`,
  name: `Profile ${index}`,
  personal: { name: `Person ${index}` },
})

test('imports every legacy profile even when the list exceeds ten', async () => {
  const legacy = Array.from({ length: 13 }, (_, index) => profile(index))
  const imported = []
  const result = await importLegacyProfiles(legacy, [], async (_url, request) => {
    const payload = JSON.parse(request.body)
    imported.push(payload.legacy_import_id)
    return { legacy_import_id: payload.legacy_import_id, data: payload.data }
  })
  assert.equal(result.complete, true)
  assert.equal(imported.length, 13)
  assert.equal(imported[12], 'legacy-12')
})

test('reports a network failure and preserves remaining legacy imports for retry', async () => {
  let calls = 0
  const result = await importLegacyProfiles([profile(1), profile(2)], [], async () => {
    calls++
    if (calls === 2) throw new TypeError('Failed to fetch')
    return { legacy_import_id: 'legacy-1', data: profile(1) }
  })
  assert.equal(result.complete, false)
  assert.match(result.error.message, /Failed to fetch/)
  assert.equal(calls, 2)
  assert.deepEqual(result.remaining.map(item => item.legacyImportId), ['legacy-2'])
})

test('keeps previously imported profiles out of a retry and preserves failed and later profiles', async () => {
  const legacy = [profile(1), profile(2), profile(3)]
  const current = [{ legacy_import_id: 'legacy-1' }]
  const result = await importLegacyProfiles(legacy, current, async (_url, request) => {
    const payload = JSON.parse(request.body)
    if (payload.legacy_import_id === 'legacy-2') throw new TypeError('offline')
    return { legacy_import_id: payload.legacy_import_id, data: payload.data }
  })
  assert.deepEqual(result.remaining.map(item => item.legacyImportId), ['legacy-2', 'legacy-3'])
})

test('profile comparison ignores object key ordering but preserves array order', () => {
  assert.equal(stableJSON({ b: 2, a: { y: 1, x: 3 } }), stableJSON({ a: { x: 3, y: 1 }, b: 2 }))
  assert.notEqual(stableJSON({ items: [1, 2] }), stableJSON({ items: [2, 1] }))
})
