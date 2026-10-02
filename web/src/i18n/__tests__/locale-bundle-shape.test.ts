import { describe, expect, it } from 'vitest'

import zhCN from '../locales/zh.json'
import zhTW from '../locales/zh-TW.json'

// The locale files wrap their strings in a `translation` object, because that is
// what i18next unwraps from a static `import`. Config.ts loads them on demand
// instead, and `addResourceBundle` expects that namespace's contents directly —
// passing the whole module nests every string one level too deep, so `t()` finds
// nothing and the UI silently falls back to English. These assertions pin the
// shape the runtime relies on.
describe('locale bundle shape', () => {
  it('keeps every string inside translation', () => {
    expect(typeof zhCN.translation).toBe('object')
    expect(Object.keys(zhCN.translation).length).toBeGreaterThan(100)
  })

  it('resolves a Chinese string from the translation namespace', () => {
    const key = 'Home'
    expect(key in zhCN.translation).toBe(true)
    expect(zhCN.translation[key]).not.toBe(key)
    // The same lookup against the module root is what the bug regressed to.
    expect((zhCN as Record<string, unknown>)[key]).toBeUndefined()
  })

  it('gives zh-TW its own bundle rather than reusing zh-CN', () => {
    expect(typeof zhTW.translation).toBe('object')
    const zhKey = 'Home'
    const twKey = 'Home'
    expect(zhCN.translation[zhKey]).not.toBe(zhTW.translation[twKey])
  })
})