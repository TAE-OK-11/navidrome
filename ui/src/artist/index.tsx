import { lazy } from 'react'
import DynamicMenuIcon from '../layout/DynamicMenuIcon'
import MicNoneOutlinedIcon from '@mui/icons-material/MicNoneOutlined'
import MicIcon from '@mui/icons-material/Mic'

export default {
  list: lazy(() => import('./ArtistList')),
  show: lazy(() => import('./ArtistShow')),
  icon: (
    <DynamicMenuIcon
      path={'artist'}
      icon={MicNoneOutlinedIcon}
      activeIcon={MicIcon}
    />
  ),
}
