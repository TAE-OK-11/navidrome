// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import PropTypes from 'prop-types'
import Avatar from '@mui/material/Avatar'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemAvatar from '@mui/material/ListItemAvatar'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemText from '@mui/material/ListItemText'
import Box from '@mui/material/Box'
import { styled } from '@mui/material/styles'
import { Link } from 'react-router-dom'
import {
  sanitizeListRestProps,
  useListContext,
  useResourceContext,
} from 'react-admin'
import { linkToRecord } from '../utils/linkToRecord'

const ListLink = styled(Link)({
  textDecoration: 'none',
  color: 'inherit',
})

const LinkOrNot = ({
  classes: classesOverride,
  linkType,
  basePath,
  id,
  record,
  children,
}) => {
  return linkType === 'edit' || linkType === true ? (
    <ListLink to={linkToRecord(basePath, id)} className={classesOverride?.link}>
      {children}
    </ListLink>
  ) : linkType === 'show' ? (
    <ListLink
      to={`${linkToRecord(basePath, id)}/show`}
      className={classesOverride?.link}
    >
      {children}
    </ListLink>
  ) : typeof linkType === 'function' ? (
    <span onClick={() => linkType(id, basePath, record)}>{children}</span>
  ) : (
    <span>{children}</span>
  )
}

export const SimpleList = ({
  basePath,
  className,
  classes: classesOverride,
  hasBulkActions = false,
  leftAvatar,
  leftIcon,
  linkType = 'edit',
  primaryText,
  rightAvatar,
  rightIcon,
  secondaryText,
  tertiaryText,
  ...rest
}) => {
  const { data = [], isPending, total = 0 } = useListContext()
  const resource = useResourceContext()
  const resolvedBasePath = basePath || `/${resource}`
  return (
    (isPending || total > 0) && (
      <List className={className} {...sanitizeListRestProps(rest)}>
        {data.map((record) => (
          <LinkOrNot
            linkType={linkType}
            basePath={resolvedBasePath}
            id={record.id}
            key={record.id}
            record={record}
          >
            <ListItemButton disableRipple={!linkType}>
              {leftIcon && (
                <ListItemIcon>{leftIcon(record, record.id)}</ListItemIcon>
              )}
              {leftAvatar && (
                <ListItemAvatar>
                  <Avatar>{leftAvatar(record, record.id)}</Avatar>
                </ListItemAvatar>
              )}
              <ListItemText
                primary={
                  <div>
                    {primaryText(record, record.id)}
                    {tertiaryText && (
                      <Box
                        component="span"
                        className={classesOverride?.tertiary}
                        sx={{ float: 'right', opacity: 0.541176 }}
                      >
                        {tertiaryText(record, record.id)}
                      </Box>
                    )}
                  </div>
                }
                secondary={secondaryText && secondaryText(record, record.id)}
              />
              {(rightAvatar || rightIcon) && (
                <ListItemSecondaryAction>
                  {rightAvatar && (
                    <Avatar>{rightAvatar(record, record.id)}</Avatar>
                  )}
                  {rightIcon && (
                    <ListItemIcon>{rightIcon(record, record.id)}</ListItemIcon>
                  )}
                </ListItemSecondaryAction>
              )}
            </ListItemButton>
          </LinkOrNot>
        ))}
      </List>
    )
  )
}

SimpleList.propTypes = {
  basePath: PropTypes.string,
  className: PropTypes.string,
  classes: PropTypes.object,
  data: PropTypes.object,
  hasBulkActions: PropTypes.bool.isRequired,
  ids: PropTypes.array,
  leftAvatar: PropTypes.func,
  leftIcon: PropTypes.func,
  linkType: PropTypes.oneOfType([
    PropTypes.string,
    PropTypes.bool,
    PropTypes.func,
  ]).isRequired,
  onToggleItem: PropTypes.func,
  primaryText: PropTypes.func,
  rightAvatar: PropTypes.func,
  rightIcon: PropTypes.func,
  secondaryText: PropTypes.func,
  selectedIds: PropTypes.arrayOf(PropTypes.any).isRequired,
  tertiaryText: PropTypes.func,
}
