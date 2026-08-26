import React, { type ComponentProps, useCallback, useMemo } from 'react'
import {
  ListPaginationContext,
  Pagination as RAPagination,
  useListPaginationContext,
} from 'react-admin'
import { setStoredPerPage, defaultRowsPerPageOptions } from './perPageStore'

type PaginationProps = ComponentProps<typeof RAPagination>

export const Pagination = ({
  rowsPerPageOptions = defaultRowsPerPageOptions,
  ...props
}: PaginationProps) => {
  const pagination = useListPaginationContext()
  const { resource, setPerPage } = pagination
  // Persist only a selector-driven change: mount, URL params and responsive
  // fallbacks never call setPerPage, so they can't overwrite the preference.
  const handleSetPerPage = useCallback(
    (value: number) => {
      if (resource) setStoredPerPage(resource, value)
      setPerPage(value)
    },
    [resource, setPerPage],
  )
  const persistedPagination = useMemo(
    () => ({ ...pagination, setPerPage: handleSetPerPage }),
    [handleSetPerPage, pagination],
  )
  return (
    <ListPaginationContext.Provider value={persistedPagination}>
      <RAPagination rowsPerPageOptions={rowsPerPageOptions} {...props} />
    </ListPaginationContext.Provider>
  )
}
