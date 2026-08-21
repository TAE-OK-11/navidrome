import { createDecisionService } from './decisionService'
import { fetchTranscodeDecision } from './fetchDecision'

export { detectBrowserProfile } from './browserProfile'
export type {
  AudioCodec,
  BrowserProfile,
  DirectPlayProfile,
  TranscodeDecision,
  TranscodingProfile,
} from './types'

export const decisionService = createDecisionService(fetchTranscodeDecision)
