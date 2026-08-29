import type { PersistedState } from '../types/redux'

export const loadState = (): PersistedState | undefined => {
  try {
    const serializedState = localStorage.getItem('state')
    if (serializedState === null) {
      return undefined
    }
    return JSON.parse(serializedState) as PersistedState
  } catch {
    return undefined
  }
}

export const saveState = (state: PersistedState): void => {
  try {
    const serializedState = JSON.stringify(state)
    localStorage.setItem('state', serializedState)
  } catch {
    // Ignore write errors
  }
}
