import React, { useState, useEffect } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { setOmittedFields, setToggleableFields } from '../actions'
import type { AppState } from '../types/redux'

type ToggleableColumns = Record<string, React.ReactNode>

type UseSelectedFieldsParams = {
  resource: string
  columns: ToggleableColumns
  omittedColumns?: string[]
  defaultOff?: string[]
}

// TODO Refactor
export const useSelectedFields = ({
  resource,
  columns,
  omittedColumns = [],
  defaultOff = [],
}: UseSelectedFieldsParams) => {
  const dispatch = useDispatch()
  const resourceFields = useSelector(
    (state: AppState) => state.settings.toggleableFields,
  )?.[resource]
  const omittedFields = useSelector(
    (state: AppState) => state.settings.omittedFields,
  )?.[resource]

  const [filteredComponents, setFilteredComponents] = useState<React.ReactNode[]>(
    [],
  )

  useEffect(() => {
    if (
      !resourceFields ||
      Object.keys(resourceFields).length !== Object.keys(columns).length ||
      !Object.keys(columns).every((c) => c in resourceFields)
    ) {
      const obj: Record<string, boolean> = {}
      for (const key of Object.keys(columns)) {
        obj[key] = !defaultOff.includes(key)
      }
      dispatch(setToggleableFields({ [resource]: obj }))
    }
    if (!omittedFields) {
      dispatch(setOmittedFields({ [resource]: omittedColumns }))
    }
  }, [
    columns,
    defaultOff,
    dispatch,
    omittedColumns,
    omittedFields,
    resource,
    resourceFields,
  ])

  useEffect(() => {
    if (resourceFields) {
      const filtered: React.ReactNode[] = []
      const omitted = [...omittedColumns]
      for (const [key, val] of Object.entries(columns)) {
        if (!val) omitted.push(key)
        else if (resourceFields[key]) filtered.push(val)
      }
      if (filteredComponents.length !== filtered.length)
        setFilteredComponents(filtered)
      if (omittedFields && omittedFields.length !== omitted.length)
        dispatch(setOmittedFields({ [resource]: omitted }))
    }
  }, [
    resourceFields,
    columns,
    dispatch,
    omittedColumns,
    omittedFields,
    resource,
    filteredComponents.length,
  ])

  return React.Children.toArray(filteredComponents)
}

export const useSetToggleableFields = (
  resource: string,
  toggleableColumns: string[],
  defaultOff: string[] = [],
) => {
  const current = useSelector(
    (state: AppState) => state.settings.toggleableFields,
  )?.album
  const dispatch = useDispatch()
  useEffect(() => {
    if (!current) {
      dispatch(
        setToggleableFields({
          [resource]: toggleableColumns.reduce<Record<string, boolean>>(
            (acc, cur) => ({
              ...acc,
              [cur]: true,
            }),
            {},
          ),
        }),
      )
      dispatch(setOmittedFields({ [resource]: defaultOff }))
    }
  }, [resource, toggleableColumns, dispatch, current, defaultOff])
}
