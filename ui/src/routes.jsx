import React, { lazy, Suspense } from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'

const HotCacheAdmin = lazy(() => import('./hotcache/HotCacheAdmin'))

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route
    exact
    path="/admin/hot-cache"
    render={() => (
      <Suspense fallback={null}>
        <HotCacheAdmin />
      </Suspense>
    )}
    key={'hot-cache'}
  />,
]

export default routes
