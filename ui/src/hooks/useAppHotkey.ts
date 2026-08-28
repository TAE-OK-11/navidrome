import { useHotkeys } from 'react-hotkeys-hook'
import type { Options } from 'react-hotkeys-hook'
import { keyMap, type HotkeyId } from '../hotkeys'

export const useAppHotkey = (
  id: HotkeyId,
  callback: (event: KeyboardEvent) => void,
  options?: Options,
) => {
  useHotkeys(keyMap[id].sequence, callback, {
    preventDefault: true,
    enableOnFormTags: false,
    ...options,
  })
}
