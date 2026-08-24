import React, { lazy, Suspense } from 'react'
import { CustomRoutes } from 'react-admin'
import { Route } from 'react-router-dom'

const Personal = lazy(() => import('./personal/Personal'))

const AppRoutes = () => (
  <CustomRoutes>
    <Route
      path="/personal"
      element={
        <Suspense fallback={null}>
          <Personal />
        </Suspense>
      }
    />
  </CustomRoutes>
)

export default AppRoutes
