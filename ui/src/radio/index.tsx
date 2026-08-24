import { lazy } from 'react'
import DynamicMenuIcon from '../layout/DynamicMenuIcon'
import RadioIcon from '@mui/icons-material/Radio'
import RadioOutlinedIcon from '@mui/icons-material/RadioOutlined'

const all = {
  list: lazy(() => import('./RadioList')),
  icon: (
    <DynamicMenuIcon
      path={'radio'}
      icon={RadioOutlinedIcon}
      activeIcon={RadioIcon}
    />
  ),
}

const admin = {
  ...all,
  create: lazy(() => import('./RadioCreate')),
  edit: lazy(() => import('./RadioEdit')),
}

export default { all, admin }
