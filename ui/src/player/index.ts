import { lazy } from 'react'
import { BsFillMusicPlayerFill } from 'react-icons/bs'

const PlayerList = lazy(() => import('./PlayerList'))
const PlayerEdit = lazy(() => import('./PlayerEdit'))

export default {
  list: PlayerList,
  edit: PlayerEdit,
  icon: BsFillMusicPlayerFill,
}
