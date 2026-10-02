/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import i18n from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage } from './languages'
import en from './locales/en.json'

// Only English is bundled. The other six locales are imported on demand, which
// keeps 3.5 MB of JSON out of the initial chunk: a visitor reads one language,
// and static imports shipped all seven.
export const resources = {
  en,
} as const

const localeLoaders: Record<string, () => Promise<{ default: object }>> = {
  fr: () => import('./locales/fr.json'),
  ja: () => import('./locales/ja.json'),
  ru: () => import('./locales/ru.json'),
  vi: () => import('./locales/vi.json'),
  zhCN: () => import('./locales/zh.json'),
  zhTW: () => import('./locales/zh-TW.json'),
}

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    supportedLngs: ['en', 'zhCN', 'fr', 'ru', 'ja', 'vi', 'zhTW'],
    load: 'currentOnly',
    nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
    debug: import.meta.env.DEV,
    interpolation: {
      escapeValue: false, // not needed for react as it escapes by default
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      // Browsers report `zh-CN`/`zh-TW`/`zh`; map them onto our `zhCN`/`zhTW`
      // codes (non-Chinese codes pass through for normal supportedLngs matching).
      convertDetectedLanguage,
    },
  })

// A language change has to pull its bundle in before i18next re-renders, or the
// UI would fall back to English for one frame on every switch. The bundled
// English needs no load, and an unknown code falls back the same way.
const loaded = new Set<string>(['en'])

function loadLocale(language: string): Promise<void> {
  const loader = localeLoaders[language]
  if (!loader || loaded.has(language)) return Promise.resolve()
  loaded.add(language)
  return loader().then((module) => {
    i18n.addResourceBundle(language, 'translation', module.default, true, true)
    // The detector may resolve after init (it reads localStorage), and init
    // itself does not emit languageChanged, so the first non-English bundle is
    // requested here as well as on every later switch.
    if (i18n.language === language) {
      i18n.changeLanguage(language)
    }
  })
}

i18n.on('languageChanged', (language: string) => {
  void loadLocale(language)
})

loadLocale(i18n.language)
if (i18n.language !== i18n.resolvedLanguage) {
  void loadLocale(i18n.resolvedLanguage ?? 'en')
}

export default i18n
