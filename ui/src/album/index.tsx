import { lazy } from 'react'
import AlbumList from './AlbumList'

export default {
  list: AlbumList,
  show: lazy(() => import('./AlbumShow')),
}
