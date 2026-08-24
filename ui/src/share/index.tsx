import { lazy } from 'react'
import ShareIcon from '@mui/icons-material/Share'
import ShareOutlinedIcon from '@mui/icons-material/ShareOutlined'
import DynamicMenuIcon from '../layout/DynamicMenuIcon'

export default {
  list: lazy(() => import('./ShareList')),
  edit: lazy(() =>
    import('./ShareEdit').then((module) => ({ default: module.ShareEdit })),
  ),
  icon: (
    <DynamicMenuIcon
      path={'share'}
      icon={ShareOutlinedIcon}
      activeIcon={ShareIcon}
    />
  ),
}
