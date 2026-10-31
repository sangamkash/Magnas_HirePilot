export const stableJSON = value => JSON.stringify(value, (_, item) =>
  item && typeof item === 'object' && !Array.isArray(item)
    ? Object.fromEntries(Object.entries(item).sort(([a], [b]) => a.localeCompare(b)))
    : item,
)

export async function importLegacyProfiles(legacy, current, api) {
  const imported = new Set(current.map(profile => profile.legacy_import_id).filter(Boolean))
  const remaining = []
  for (const [index, item] of legacy.entries()) {
    if (imported.has(item.legacyImportId)) continue
    try {
      const result = await api('/api/v1/profiles', {
        method: 'POST',
        body: JSON.stringify({
          name: item.name || item.personal?.headline || item.personal?.name || 'My profile',
          data: item,
          ...(item.legacyImportId ? { legacy_import_id: item.legacyImportId } : {}),
        }),
      })
      if (!result || result.legacy_import_id !== item.legacyImportId || stableJSON(result.data) !== stableJSON(item)) {
        remaining.push(...legacy.slice(index))
        return { complete: false, remaining, error: new Error('Imported profile data did not match the saved copy.') }
      }
    } catch (error) {
      remaining.push(...legacy.slice(index))
      return { complete: false, remaining, error }
    }
  }
  return { complete: true, remaining: [] }
}
