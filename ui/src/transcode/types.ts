export type AudioCodec =
  'mp3' | 'opus' | 'vorbis' | 'flac' | 'wav' | 'alac' | 'aac'

export interface DirectPlayProfile {
  containers: string[]
  audioCodecs: AudioCodec[]
  protocols: string[]
}

export interface TranscodingProfile {
  container: string
  audioCodec: AudioCodec
  protocol: string
}

export interface BrowserProfile {
  name: string
  platform: string
  directPlayProfiles: DirectPlayProfile[]
  transcodingProfiles: TranscodingProfile[]
  codecProfiles: unknown[]
}

export interface SourceStream {
  codec?: string
  container?: string
  [key: string]: unknown
}

export interface TranscodeDecision {
  canDirectPlay?: boolean
  canTranscode?: boolean
  transcodeParams?: string
  sourceStream?: SourceStream
  [key: string]: unknown
}

export type FetchTranscodeDecision = (
  songId: string,
  browserProfile: BrowserProfile,
) => Promise<TranscodeDecision | null>
