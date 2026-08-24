import { lazy } from 'react'
import QueueMusicOutlinedIcon from '@mui/icons-material/QueueMusicOutlined'
import QueueMusicIcon from '@mui/icons-material/QueueMusic'
import DynamicMenuIcon from '../layout/DynamicMenuIcon'

export default {
  list: lazy(() => import('./PlaylistList')),
  create: lazy(() => import('./PlaylistCreate')),
  edit: lazy(() => import('./PlaylistEdit')),
  show: lazy(() => import('./PlaylistShow')),
  icon: (
    <DynamicMenuIcon
      path={'playlist'}
      icon={QueueMusicOutlinedIcon}
      activeIcon={QueueMusicIcon}
    />
  ),
}
