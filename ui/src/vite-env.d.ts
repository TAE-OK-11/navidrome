/// <reference types="vite/client" />
/// <reference types="vite-plugin-pwa/client" />

import type { StoreEnhancer } from 'redux'

declare global {
  interface Window {
    __REDUX_DEVTOOLS_EXTENSION_COMPOSE__?: (
      options?: { trace?: boolean; traceLimit?: number },
    ) => StoreEnhancer
  }
}

export {}
