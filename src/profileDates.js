export function toISODate(value) {
  if (!value) return ''
  const parts = value.match(/^(\d{2})\/(\d{2})\/(\d{4})$/)
  if (parts) {
    const [, day, month, year] = parts
    const date = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)))
    return date.getUTCFullYear() === Number(year) && date.getUTCMonth() === Number(month) - 1 && date.getUTCDate() === Number(day)
      ? `${year}-${month}-${day}`
      : ''
  }
  const month = value.match(/^(\d{4})-(\d{2})$/)
  if (month) return Number(month[2]) >= 1 && Number(month[2]) <= 12 ? `${month[1]}-${month[2]}-01` : ''
  return /^\d{4}-\d{2}-\d{2}$/.test(value) && !Number.isNaN(Date.parse(`${value}T00:00:00Z`)) ? value : ''
}

export function fromISODate(value) {
  if (!value) return ''
  const [year, month, day] = value.split('-')
  return `${day}/${month}/${year}`
}

export function normalizeProfileDates(profile) {
  const normalizeRows = (rows, fields) => (rows || []).map(entry => ({
    ...entry,
    ...Object.fromEntries(fields.map(field => [field, toISODate(entry[field])])),
  }))
  return {
    ...profile,
    education: normalizeRows(profile.education, ['startDate', 'endDate']),
    experience: normalizeRows(profile.experience, ['startDate', 'endDate']),
    certificates: normalizeRows(profile.certificates, ['issueDate', 'expirationDate']),
    awards: normalizeRows(profile.awards, ['date']),
  }
}
