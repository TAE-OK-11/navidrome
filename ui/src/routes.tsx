import React, { lazy, Suspense } from 'react'
import { CustomRoutes } from 'react-admin'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'

const HotCacheAdmin = lazy(() => import('./hotcache/HotCacheAdmin'))

const AppRoutes = () => (
  <CustomRoutes>
    <Route path="/personal" element={<Personal />} />
    <Route
      path="/admin/hot-cache"
      element={
        <Suspense fallback={null}>
          <HotCacheAdmin />
        </Suspense>
      }
    />
  </CustomRoutes>
)

export default AppRoutes
