import { lazy } from 'react'

const UserList = lazy(() => import('./UserList'))
const UserEdit = lazy(() => import('./UserEdit'))
const UserCreate = lazy(() => import('./UserCreate'))

export default {
  list: UserList,
  edit: UserEdit,
  create: UserCreate,
}
