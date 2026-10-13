/** Text shown wherever a value is missing. Never leave a blank on screen. */
export const UNKNOWN = 'unknown'

const UNITS = ['bytes', 'KB', 'MB', 'GB', 'TB', 'PB'] as const

/**
 * Formats a byte count the way hardware is sold (1 GB = 1024 MB), so an
 * 80 GB GPU reads "80 GB" rather than "85.9 GB".
 */
export function formatBytes(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes) || bytes < 0) {
    return UNKNOWN
  }
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024
    unit += 1
  }
  const rounded = value >= 100 || unit === 0 ? Math.round(value) : Math.round(value * 10) / 10
  return `${rounded} ${UNITS[unit]}`
}

/** Returns the value as text, or "unknown" when it is missing or empty. */
export function orUnknown(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') {
    return UNKNOWN
  }
  return String(value)
}
