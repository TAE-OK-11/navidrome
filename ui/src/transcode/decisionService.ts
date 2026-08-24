// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { jwtDecode } from 'jwt-decode'
import subsonic from '../subsonic'
import { baseUrl } from '../utils'
import type {
  BrowserProfile,
  FetchTranscodeDecision,
  TranscodeDecision,
} from './types'

type JwtPayload = { exp?: unknown }
type CacheEntry = { decision: TranscodeDecision | null }

export interface DecisionService {
  getDecision(
    songId: string,
    browserProfile?: BrowserProfile | null,
  ): Promise<TranscodeDecision | null>
  getCachedDecision(songId: string): TranscodeDecision | null
  prefetchDecisions(
    songIds: string[],
    browserProfile?: BrowserProfile | null,
  ): Promise<void>
  resolveStreamUrl(songId: string): Promise<string>
  invalidateAll(): void
  buildStreamUrl(
    songId: string,
    transcodeParams: string,
    offset?: number,
  ): string
  setProfile(profile: BrowserProfile | null): void
  getProfile(): BrowserProfile | null
}

// Decode the exp claim from a JWT token. Signature verification is intentionally
// left to the server; the UI only uses exp to avoid caching a stale decision.
export function decodeJwtExp(token?: string | null): number | null {
  try {
    if (!token) return null
    const payload = jwtDecode<JwtPayload>(token)
    return typeof payload.exp === 'number' ? payload.exp : null
  } catch {
    return null
  }
}

export function createDecisionService(
  fetchFn: FetchTranscodeDecision,
): DecisionService {
  const cache = new Map<string, CacheEntry>()
  let currentProfile: BrowserProfile | null = null

  function isFresh(entry: CacheEntry): boolean {
    const exp = decodeJwtExp(entry.decision?.transcodeParams)
    if (exp == null) return false
    return Date.now() < (exp - 60) * 1000
  }

  function setProfile(profile: BrowserProfile | null): void {
    currentProfile = profile
  }

  function getProfile(): BrowserProfile | null {
    return currentProfile
  }

  async function getDecision(
    songId: string,
    browserProfile?: BrowserProfile | null,
  ): Promise<TranscodeDecision | null> {
    const profile = browserProfile ?? currentProfile
    if (!profile) return null

    const cached = cache.get(songId)
    if (cached && isFresh(cached)) return cached.decision

    const decision = await fetchFn(songId, profile)
    cache.set(songId, { decision })
    return decision
  }

  async function prefetchDecisions(
    songIds: string[],
    browserProfile?: BrowserProfile | null,
  ): Promise<void> {
    const profile = browserProfile ?? currentProfile
    if (!profile) return

    const uncached = songIds.filter((id) => {
      const entry = cache.get(id)
      return !entry || !isFresh(entry)
    })

    await Promise.allSettled(
      uncached.map(async (id) => {
        const decision = await fetchFn(id, profile)
        cache.set(id, { decision })
      }),
    )
  }

  function invalidateAll(): void {
    cache.clear()
  }

  function buildStreamUrl(
    songId: string,
    transcodeParams: string,
    offset?: number,
  ): string {
    const params: Record<string, string | number> = {
      mediaId: songId,
      mediaType: 'song',
      transcodeParams,
    }
    if (offset != null && offset > 0) params.offset = offset
    return baseUrl(subsonic.url('getTranscodeStream', null, params))
  }

  async function resolveStreamUrl(songId: string): Promise<string> {
    const decision = await getDecision(songId)
    if (!decision?.transcodeParams) {
      return baseUrl(subsonic.streamUrl(songId))
    }
    return buildStreamUrl(songId, decision.transcodeParams)
  }

  function getCachedDecision(songId: string): TranscodeDecision | null {
    const entry = cache.get(songId)
    return entry && isFresh(entry) ? entry.decision : null
  }

  return {
    getDecision,
    getCachedDecision,
    prefetchDecisions,
    resolveStreamUrl,
    invalidateAll,
    buildStreamUrl,
    setProfile,
    getProfile,
  }
}
