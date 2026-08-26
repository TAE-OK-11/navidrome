declare global {
  interface Window {
    global: typeof globalThis
  }
}

window.global = window // fix "global is not defined" error in react-image-lightbox

import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'
import { registerSW } from 'virtual:pwa-register'

registerSW({ immediate: true })

const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Root element not found')
}

createRoot(rootElement).render(<App />)
