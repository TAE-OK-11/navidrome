import { lazy } from 'react'
import { MdLibraryMusic } from 'react-icons/md'

const LibraryList = lazy(() => import('./LibraryList'))
const LibraryEdit = lazy(() => import('./LibraryEdit'))
const LibraryCreate = lazy(() => import('./LibraryCreate'))

export default {
  icon: MdLibraryMusic,
  list: LibraryList,
  edit: LibraryEdit,
  create: LibraryCreate,
}
