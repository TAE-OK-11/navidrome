import { TRANSCODING_SET_PROFILE } from '../actions'
import type { TranscodingState, UnknownAction } from '../types/redux'

const initialState: TranscodingState = {
  browserProfile: null,
}

export const transcodingReducer = (
  state: TranscodingState = initialState,
  { type, data }: UnknownAction,
): TranscodingState => {
  switch (type) {
    case TRANSCODING_SET_PROFILE:
      return { ...state, browserProfile: data ?? null }
    default:
      return state
  }
}
