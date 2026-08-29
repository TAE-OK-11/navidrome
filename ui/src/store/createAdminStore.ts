import {
  combineReducers,
  legacy_createStore as createStore,
  type Reducer,
  type StoreEnhancer,
} from 'redux'
import throttle from '../utils/throttle'
import { loadState, saveState } from './persistState'
import type { NavidromeRootState, PersistedState } from '../types/redux'

type NavidromeReducers = {
  [K in keyof NavidromeRootState]: Reducer<NavidromeRootState[K]>
}

type CreateAdminStoreOptions = {
  customReducers?: Partial<NavidromeReducers>
}

const createAdminStore = ({ customReducers = {} }: CreateAdminStoreOptions) => {
  const reducer = combineReducers(
    customReducers as NavidromeReducers,
  ) as Reducer<NavidromeRootState>

  const devToolsCompose = (
    window as Window & {
      __REDUX_DEVTOOLS_EXTENSION_COMPOSE__?: (options?: {
        trace?: boolean
        traceLimit?: number
      }) => (...funcs: StoreEnhancer[]) => StoreEnhancer
    }
  ).__REDUX_DEVTOOLS_EXTENSION_COMPOSE__

  let enhancer: StoreEnhancer | undefined
  if (import.meta.env.DEV && typeof window !== 'undefined' && devToolsCompose) {
    enhancer = (devToolsCompose({
      trace: true,
      traceLimit: 25,
    }) as unknown as () => StoreEnhancer)()
  }

  const persistedState = loadState()
  if (persistedState?.player?.savedPlayIndex != null) {
    const hydratedPlayer = persistedState.player as PersistedState['player'] & {
      playIndex?: number
    }
    hydratedPlayer.playIndex = persistedState.player.savedPlayIndex
  }

  const preloadedState = persistedState as unknown as
    | NavidromeRootState
    | undefined

  const store = enhancer
    ? createStore(reducer, preloadedState, enhancer)
    : createStore(reducer, preloadedState)

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
    }, 1000),
  )

  return store
}

export default createAdminStore
