// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useCallback, useMemo } from 'react'
import {
  useUpdate,
  useNotify,
  useRefresh,
  useRecordContext,
  useTranslate,
  useResourceContext,
} from 'react-admin'
import Switch from '@mui/material/Switch'
import { Tooltip, FormControlLabel } from '@mui/material'

const enabledSwitchSx = (theme) => {
  const color = theme.palette.success?.main || theme.palette.primary.main
  return {
    '& .MuiSwitch-colorSecondary.Mui-checked': { color },
    '& .MuiSwitch-colorSecondary.Mui-checked + .MuiSwitch-track': {
      backgroundColor: color,
    },
  }
}

const errorSwitchSx = {
  '& .MuiSwitch-thumb': { bgcolor: 'warning.main' },
  '& .MuiSwitch-track': { bgcolor: 'warning.light', opacity: 0.7 },
}

/**
 * Shared toggle switch for enabling/disabling plugins.
 * Used in both PluginList (compact) and PluginShow (with label).
 *
 * @param {Object} props
 * @param {boolean} [props.showLabel=false] - Whether to show the enable/disable label
 * @param {string} [props.size='small'] - Switch size ('small' or 'medium')
 * @param {Object} [props.manifest=null] - Parsed manifest object for permission checking
 */
const ToggleEnabledSwitch = ({
  showLabel = false,
  size = 'small',
  manifest = null,
}) => {
  const resource = useResourceContext()
  const record = useRecordContext()
  const notify = useNotify()
  const refresh = useRefresh()
  const translate = useTranslate()

  const [toggleEnabled, { isPending }] = useUpdate(
    resource,
    {
      id: record?.id,
      data: { enabled: !record?.enabled },
      previousData: record,
    },
    {
      mutationMode: 'pessimistic',
      onSuccess: () => {
        refresh()
        notify(
          record?.enabled
            ? 'resources.plugin.notifications.disabled'
            : 'resources.plugin.notifications.enabled',
          { type: 'info' },
        )
      },
      onError: (error) => {
        refresh()
        notify(error?.message || 'resources.plugin.notifications.error', {
          type: 'warning',
        })
      },
    },
  )

  const handleClick = useCallback(
    (e) => {
      e.stopPropagation()
      toggleEnabled()
    },
    [toggleEnabled],
  )

  const hasError = !!record?.lastError

  // Check if users permission is required but not configured
  const usersPermissionRequired = useMemo(() => {
    if (!manifest?.permissions?.users) return false
    if (record?.allUsers) return false
    // Check if users array is empty or not set
    if (!record?.users) return true
    try {
      const users = JSON.parse(record.users)
      return users.length === 0
    } catch {
      return true
    }
  }, [manifest, record?.allUsers, record?.users])

  // Check if library permission is required but not configured
  const libraryPermissionRequired = useMemo(() => {
    if (!manifest?.permissions?.library) return false
    if (record?.allLibraries) return false
    // Check if libraries array is empty or not set
    if (!record?.libraries) return true
    try {
      const libraries = JSON.parse(record.libraries)
      return libraries.length === 0
    } catch {
      return true
    }
  }, [manifest, record?.allLibraries, record?.libraries])

  const permissionRequired =
    usersPermissionRequired || libraryPermissionRequired
  const isDisabled =
    isPending || hasError || (permissionRequired && !record?.enabled)

  const tooltipTitle = useMemo(() => {
    if (hasError) {
      return translate('resources.plugin.actions.disabledDueToError')
    }
    if (usersPermissionRequired && !record?.enabled) {
      return translate('resources.plugin.actions.disabledUsersRequired')
    }
    if (libraryPermissionRequired && !record?.enabled) {
      return translate('resources.plugin.actions.disabledLibrariesRequired')
    }
    if (!showLabel) {
      return translate(
        record?.enabled
          ? 'resources.plugin.actions.disable'
          : 'resources.plugin.actions.enable',
      )
    }
    return ''
  }, [
    hasError,
    usersPermissionRequired,
    libraryPermissionRequired,
    showLabel,
    record?.enabled,
    translate,
  ])

  const switchElement = (
    <Switch
      checked={record?.enabled ?? false}
      onClick={handleClick}
      disabled={isDisabled}
      sx={isDisabled ? errorSwitchSx : enabledSwitchSx}
      size={size}
      color="primary"
    />
  )

  if (showLabel) {
    const showTooltip = hasError || (permissionRequired && !record?.enabled)
    return (
      <Tooltip
        title={tooltipTitle}
        disableHoverListener={!showTooltip}
        disableFocusListener={!showTooltip}
      >
        <FormControlLabel
          control={switchElement}
          label={translate(
            record?.enabled
              ? 'resources.plugin.actions.disable'
              : 'resources.plugin.actions.enable',
          )}
        />
      </Tooltip>
    )
  }

  return (
    <Tooltip title={tooltipTitle}>
      <span>{switchElement}</span>
    </Tooltip>
  )
}

export default ToggleEnabledSwitch
