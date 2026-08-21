declare module 'lodash.throttle' {
  export interface ThrottleSettings {
    leading?: boolean
    trailing?: boolean
  }

  export default function throttle(
    func: (event: Event) => void,
    wait?: number,
    options?: ThrottleSettings,
  ): (event: Event) => void
}
