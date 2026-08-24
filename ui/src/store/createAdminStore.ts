// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import {
  combineReducers,
  compose,
  legacy_createStore as createStore,
} from 'redux'
import throttle from 'lodash.throttle'
import { loadState, saveState } from './persistState'

const createAdminStore = ({ customReducers = {} }) => {
  const reducer = combineReducers(customReducers)

  const composeEnhancers =
    (process.env.NODE_ENV === 'development' &&
      typeof window !== 'undefined' &&
      window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__ &&
      window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__({
        trace: true,
        traceLimit: 25,
      })) ||
    compose

  const persistedState = loadState()
  if (persistedState?.player?.savedPlayIndex) {
    persistedState.player.playIndex = persistedState.player.savedPlayIndex
  }

  const store = createStore(reducer, persistedState, composeEnhancers())

  store.subscribe(
    throttle(() => {
      const state = store.getState()
      saveState({
        theme: state.theme,
        library: state.library,
        player: (({ queue, volume, savedPlayIndex }) => ({
          queue,
          volume,
          savedPlayIndex,
        }))(state.player),
        albumView: state.albumView,
        settings: state.settings,
      })
    }),
    1000,
  )

  return store
}

export default createAdminStore
