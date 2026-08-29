import { CHANGE_GAIN, CHANGE_PREAMP } from '../actions'
import type { ReplayGainState } from '../types/redux'

const GAIN_KEY = 'gainMode'
const PREAMP_KEY = 'preAmp'

const getPreAmp = (): number => {
  const storage = localStorage.getItem(PREAMP_KEY)

  if (storage === null) {
    return 0
  } else {
    const asFloat = parseFloat(storage)
    return isNaN(asFloat) ? 0 : asFloat
  }
}

const initialState: ReplayGainState = {
  gainMode: localStorage.getItem(GAIN_KEY) || 'none',
  preAmp: getPreAmp(),
}

type ReplayGainAction = {
  type: string
  payload?: string
}

export const replayGainReducer = (
  previousState: ReplayGainState = initialState,
  { type, payload }: ReplayGainAction,
): ReplayGainState => {
  switch (type) {
    case CHANGE_GAIN: {
      if (payload == null) {
        return previousState
      }
      localStorage.setItem(GAIN_KEY, payload)
      return {
        ...previousState,
        gainMode: payload,
      }
    }
    case CHANGE_PREAMP: {
      if (payload == null) {
        return previousState
      }
      const value = parseFloat(payload)
      if (isNaN(value)) {
        return previousState
      }
      localStorage.setItem(PREAMP_KEY, payload)
      return {
        ...previousState,
        preAmp: value,
      }
    }
    default:
      return previousState
  }
}
