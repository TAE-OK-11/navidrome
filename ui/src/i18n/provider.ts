import polyglotI18nProvider from 'ra-i18n-polyglot'
import deepmerge from 'deepmerge'
import type { TranslationMessages } from 'react-admin'
import dataProvider from '../dataProvider'
import en from './en.json'
import { i18nProvider } from './index'

type StoredTranslation = {
  id: string
  data: string
}

const parseStoredTranslation = (): StoredTranslation | null => {
  const raw = localStorage.getItem('translation')
  if (!raw) {
    return null
  }
  return JSON.parse(raw) as StoredTranslation
}

// Only returns current selected locale if its translations are found in localStorage
const defaultLocale = function () {
  const locale = localStorage.getItem('locale')
  const current = parseStoredTranslation()
  if (locale && current && current.id === locale) {
    // Asynchronously reload the translation from the server
    retrieveTranslation(locale).then(() => {
      i18nProvider.changeLocale(locale)
    })
    return locale
  }
  return 'en'
}

export function retrieveTranslation(locale: string) {
  return dataProvider.getOne('translation', { id: locale }).then((res) => {
    localStorage.setItem('translation', JSON.stringify(res.data))
    return prepareLanguage(JSON.parse(res.data.data))
  })
}

const removeEmpty = (obj: Record<string, unknown>) => {
  for (const k in obj) {
    if (
      Object.prototype.hasOwnProperty.call(obj, k) &&
      typeof obj[k] === 'object'
    ) {
      removeEmpty(obj[k] as Record<string, unknown>)
    } else {
      if (!obj[k]) {
        delete obj[k]
      }
    }
  }
}

const prepareLanguage = (lang: Record<string, unknown>): TranslationMessages => {
  removeEmpty(lang)
  // Make "albumSong" and "playlistTrack" resource use the same translations as "song"
  const resources = lang.resources as Record<string, unknown>
  resources.albumSong = resources.song
  resources.playlistTrack = resources.song
  // ra.boolean.null should always be empty
  const ra = lang.ra as Record<string, Record<string, string>>
  ra.boolean.null = ''
  // Fallback to english translations
  return deepmerge(en, lang) as TranslationMessages
}

const getMessages = (
  locale: string,
): TranslationMessages | Promise<TranslationMessages> => {
  // English is bundled
  if (locale === 'en') {
    return prepareLanguage(en as Record<string, unknown>)
  }
  // If the requested locale is in already loaded, return it
  const current = parseStoredTranslation()
  if (current && current.id === locale) {
    return prepareLanguage(JSON.parse(current.data))
  }
  // If not, get it from the server, and store it in localStorage
  return retrieveTranslation(locale)
}

export default polyglotI18nProvider(getMessages, defaultLocale())
