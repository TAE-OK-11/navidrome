import subsonic from '../subsonic'
import { httpClient } from '../dataProvider'
import type { BrowserProfile, TranscodeDecision } from './types'

interface SubsonicError {
  code?: number | string
  message?: string
}

interface TranscodeDecisionResponse {
  status: string
  error?: SubsonicError
  transcodeDecision?: TranscodeDecision
}

interface SubsonicEnvelope {
  'subsonic-response': TranscodeDecisionResponse
}

export async function fetchTranscodeDecision(
  songId: string,
  browserProfile: BrowserProfile,
): Promise<TranscodeDecision | null> {
  const fetchUrl = subsonic.url('getTranscodeDecision', null, {
    mediaId: songId,
    mediaType: 'song',
  })

  const { json } = await httpClient(fetchUrl, {
    method: 'POST',
    body: JSON.stringify(browserProfile),
  })

  const subsonicResponse = (json as SubsonicEnvelope)['subsonic-response']
  if (!subsonicResponse || subsonicResponse.status !== 'ok') {
    const error = subsonicResponse?.error ?? {}
    throw new Error(
      `getTranscodeDecision error: ${error.code ?? 'unknown'} ${error.message ?? 'unknown error'}`,
    )
  }

  return subsonicResponse.transcodeDecision ?? null
}
