import { lazy } from 'react'
import { GrDocumentMissing } from 'react-icons/gr'

const MissingList = lazy(() => import('./MissingFilesList'))
export default {
  list: MissingList,
  icon: GrDocumentMissing,
}
