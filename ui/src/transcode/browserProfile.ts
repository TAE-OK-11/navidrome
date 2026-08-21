import type {
  AudioCodec,
  BrowserProfile,
  DirectPlayProfile,
  TranscodingProfile,
} from './types'

interface CodecProbe {
  codec: AudioCodec
  container: string
  mime: readonly string[]
}

// Each entry: { codec name for the server, container, mime: [MIME probe strings] }
export const CODEC_PROBES: readonly CodecProbe[] = [
  { codec: 'mp3', container: 'mp3', mime: ['audio/mpeg; codecs="mp3"'] },
  { codec: 'opus', container: 'ogg', mime: ['audio/ogg; codecs="opus"'] },
  { codec: 'vorbis', container: 'ogg', mime: ['audio/ogg; codecs="vorbis"'] },
  {
    codec: 'flac',
    container: 'flac',
    mime: ['audio/flac', 'audio/flac; codecs="flac"'],
  },
  { codec: 'wav', container: 'wav', mime: ['audio/wav; codecs="1"'] },
  { codec: 'alac', container: 'mp4', mime: ['audio/mp4; codecs="alac"'] },
  { codec: 'aac', container: 'mp4', mime: ['audio/mp4; codecs="mp4a.40.2"'] },
]

const TRANSCODE_CODECS: readonly AudioCodec[] = ['flac', 'opus', 'mp3']
const SAFARI_TRANSCODE_CODECS: readonly AudioCodec[] = ['mp3']

function canPlay(
  audio: HTMLAudioElement,
  mimeList: readonly string[],
): boolean {
  return mimeList.some((mime) => {
    const result = audio.canPlayType(mime)
    return result === 'probably' || result === 'maybe'
  })
}

function probeSupported(
  audio: HTMLAudioElement,
  probes: readonly CodecProbe[],
): CodecProbe[] {
  return probes.filter(({ mime }) => canPlay(audio, mime))
}

function isSafari(): boolean {
  const ua = navigator.userAgent
  return (
    ua.includes('Safari') && !ua.includes('Chrome') && !ua.includes('Chromium')
  )
}

export function detectBrowserProfile(): BrowserProfile {
  const audio = new Audio()

  const directPlayProfiles: DirectPlayProfile[] = probeSupported(
    audio,
    CODEC_PROBES,
  ).map(({ codec, container }) => ({
    containers: [container],
    audioCodecs: [codec],
    protocols: ['http'],
  }))

  const transcodeCodecs = isSafari()
    ? SAFARI_TRANSCODE_CODECS
    : TRANSCODE_CODECS
  const transcodingProfiles = transcodeCodecs.reduce<TranscodingProfile[]>(
    (profiles, codec) => {
      const probe = CODEC_PROBES.find((candidate) => candidate.codec === codec)
      if (!probe) return profiles
      if (canPlay(audio, probe.mime) || codec === 'mp3') {
        profiles.push({
          container: probe.container,
          audioCodec: codec,
          protocol: 'http',
        })
      }
      return profiles
    },
    [],
  )

  return {
    name: 'NavidromeUI',
    platform: navigator.userAgent,
    directPlayProfiles,
    transcodingProfiles,
    codecProfiles: [],
  }
}
