import { useSelector } from 'react-redux'
import { useEffect, useRef } from 'react'
import { useRefresh, useDataProvider } from 'react-admin'
import type { NavidromeRootState } from '../types/redux'

/**
 * A hook that automatically refreshes react-admin managed resources when refresh events are received via SSE.
 *
 * This hook is designed for components that display react-admin managed resources (like lists, shows, edits)
 * and need to stay in sync when those resources are modified elsewhere in the application.
 *
 * **When to use this hook:**
 * - Your component displays react-admin resources (albums, songs, artists, playlists, etc.)
 * - You want automatic refresh when those resources are created/updated/deleted
 * - Your data comes from standard dataProvider.getMany() calls
 * - You're using react-admin's data management (queries, mutations, caching)
 *
 * **When NOT to use this hook:**
 * - Your component displays derived/custom data not directly managed by react-admin
 * - You need custom reload logic beyond dataProvider.getMany()
 * - Your data comes from non-standard endpoints
 * - Use `useRefreshOnEvents` instead for these scenarios
 *
 * @param {...string} visibleResources - Resource names to watch for changes.
 *   If no resources specified, watches all resources.
 *   If '*' is included in resources, triggers full page refresh.
 *
 * @example
 * // Example 1: Album list - refresh when albums change
 * const AlbumList = () => {
 *   useResourceRefresh('album')
 *   return <List resource="album">...</List>
 * }
 *
 * @example
 * // Example 2: Album show page - refresh when album or its songs change
 * const AlbumShow = () => {
 *   useResourceRefresh('album', 'song')
 *   return <Show resource="album">...</Show>
 * }
 *
 * @example
 * // Example 3: Dashboard - refresh when any resource changes
 * const Dashboard = () => {
 *   useResourceRefresh() // No parameters = watch all resources
 *   return <div>...</div>
 * }
 *
 * @example
 * // Example 4: Library management page - watch library resources
 * const LibraryList = () => {
 *   useResourceRefresh('library')
 *   return <List resource="library">...</List>
 * }
 *
 * **How it works:**
 * - Listens to refresh events from the SSE connection
 * - When events arrive, checks if they match the specified visible resources
 * - For specific resource IDs: calls dataProvider.getMany(resource, {ids: [...]})
 * - For global refreshes: calls refresh() to reload the entire page
 * - Uses react-admin's built-in data management and caching
 *
 * **Event format expected:**
 * - Global refresh: { '*': '*' } or { someResource: ['*'] }
 * - Specific resources: { album: ['id1', 'id2'], song: ['id3'] }
 */
export const useResourceRefresh = (...visibleResources) => {
  const lastTime = useRef(Date.now())
  const visibleResourcesRef = useRef(visibleResources)
  visibleResourcesRef.current = visibleResources
  const refresh = useRefresh()
  const dataProvider = useDataProvider()
  const refreshData = useSelector(
    (state: NavidromeRootState) => state.activity?.refresh,
  )

  useEffect(() => {
    const { resources, lastReceived = 0 } = refreshData || {}
    if (lastReceived <= lastTime.current) {
      return
    }
    lastTime.current = lastReceived

    if (
      resources &&
      (resources['*'] === '*' ||
        Object.values(resources).some(
          (values) =>
            values === '*' ||
            (Array.isArray(values) && values.includes('*')),
        ))
    ) {
      refresh()
      return
    }
    if (resources) {
      Object.keys(resources).forEach((resource) => {
        const watched = visibleResourcesRef.current
        if (watched.length === 0 || watched.includes(resource)) {
          const ids = resources[resource]
          if (Array.isArray(ids) && ids.length > 0) {
            dataProvider.getMany(resource, { ids })
          }
        }
      })
    }
  }, [dataProvider, refresh, refreshData])
}
