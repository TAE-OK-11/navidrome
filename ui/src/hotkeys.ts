import config from './config'

export type HotkeyDefinition = {
  name: string
  sequence: string
  group: 'Global' | 'Player'
}

export type HotkeyId = keyof typeof keyMap

const keyMap = {
  SHOW_HELP: { name: 'show_help', sequence: 'shift+?', group: 'Global' },
  TOGGLE_MENU: { name: 'toggle_menu', sequence: 'm', group: 'Global' },
  TOGGLE_PLAY: { name: 'toggle_play', sequence: 'space', group: 'Player' },
  PREV_SONG: { name: 'prev_song', sequence: 'left', group: 'Player' },
  NEXT_SONG: { name: 'next_song', sequence: 'right', group: 'Player' },
  CURRENT_SONG: { name: 'current_song', sequence: 'shift+c', group: 'Player' },
  VOL_UP: { name: 'vol_up', sequence: '=', group: 'Player' },
  VOL_DOWN: { name: 'vol_down', sequence: '-', group: 'Player' },
  TOGGLE_LOVE: { name: 'toggle_love', sequence: 'l', group: 'Player' },
} as const satisfies Record<string, HotkeyDefinition>

export const hotkeyEntries = Object.entries(keyMap)
  .filter(([id]) => id !== 'TOGGLE_LOVE' || config.enableFavourites)
  .map(([id, entry]) => ({ id, ...entry }))

export { keyMap }
