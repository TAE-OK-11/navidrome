import { lazy } from 'react'
import { VscExtensions } from 'react-icons/vsc'

const PluginList = lazy(() => import('./PluginList'))
const PluginShow = lazy(() => import('./PluginShow'))

export default {
  icon: VscExtensions,
  list: PluginList,
  show: PluginShow,
}
