import { describe, expect, it } from 'vitest'
import { formatBytes, orUnknown } from './format'

describe('formatBytes', () => {
  it.each([
    [0, '0 bytes'],
    [512, '512 bytes'],
    [1024, '1 KB'],
    [1536, '1.5 KB'],
    [85899345920, '80 GB'],
    [549755813888, '512 GB'],
    [1099511627776, '1 TB'],
    [25769803776, '24 GB'],
    [null, 'unknown'],
    [undefined, 'unknown'],
    [-1, 'unknown'],
    [Number.NaN, 'unknown'],
  ])('formats %s as %s', (input, want) => {
    expect(formatBytes(input)).toBe(want)
  })
})

describe('orUnknown', () => {
  it.each([
    ['NVIDIA A100', 'NVIDIA A100'],
    [0, '0'],
    ['', 'unknown'],
    [null, 'unknown'],
    [undefined, 'unknown'],
  ])('shows %s as %s', (input, want) => {
    expect(orUnknown(input)).toBe(want)
  })
})
