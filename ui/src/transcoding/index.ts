import { lazy } from 'react'
import { MdTransform } from 'react-icons/md'
import config from '../config'

const TranscodingList = lazy(() => import('./TranscodingList'))
const TranscodingEdit = lazy(() => import('./TranscodingEdit'))
const TranscodingCreate = lazy(() => import('./TranscodingCreate'))
const TranscodingShow = lazy(() => import('./TranscodingShow'))

export default {
  list: TranscodingList,
  edit: config.enableTranscodingConfig && TranscodingEdit,
  create: config.enableTranscodingConfig && TranscodingCreate,
  show: !config.enableTranscodingConfig && TranscodingShow,
  icon: MdTransform,
}
