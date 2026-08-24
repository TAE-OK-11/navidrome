type GainInfo = {
  gainMode: string
  preAmp?: number
}

type ReplayGainRecord = {
  rgAlbumGain?: number
  rgAlbumPeak?: number
  rgTrackGain?: number
  rgTrackPeak?: number
}

const calculateReplayGain = (preAmp: number, gain?: number, peak?: number) => {
  if (gain === undefined || peak === undefined) {
    return 1
  }

  // https://wiki.hydrogenaud.io/index.php?title=ReplayGain_1.0_specification&section=19
  // Normalized to max gain
  return Math.min(10 ** ((gain + preAmp) / 20), 1 / peak)
}

export const calculateGain = (gainInfo: GainInfo, song: ReplayGainRecord) => {
  const preAmp = gainInfo.preAmp ?? 0
  switch (gainInfo.gainMode) {
    case 'album': {
      return calculateReplayGain(preAmp, song.rgAlbumGain, song.rgAlbumPeak)
    }
    case 'track': {
      return calculateReplayGain(preAmp, song.rgTrackGain, song.rgTrackPeak)
    }
    default: {
      return 1
    }
  }
}
