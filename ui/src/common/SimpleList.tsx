import React from 'react'
import Avatar from '@mui/material/Avatar'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemAvatar from '@mui/material/ListItemAvatar'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemText from '@mui/material/ListItemText'
import Box from '@mui/material/Box'
import { Link } from 'react-router-dom'
import {
  sanitizeListRestProps,
  useListContext,
  useResourceContext,
} from 'react-admin'
import { linkToRecord } from '../utils/linkToRecord'

type SimpleRecord = { id: string | number; [key: string]: unknown }
type SimpleRenderer = (
  record: SimpleRecord,
  id: string | number,
) => React.ReactNode
type SimpleListProps = {
  basePath?: string
  className?: string
  classes?: { link?: string; tertiary?: string }
  hasBulkActions?: boolean
  leftAvatar?: SimpleRenderer
  leftIcon?: SimpleRenderer
  linkType?:
    | 'edit'
    | 'show'
    | boolean
    | ((id: string | number, basePath: string, record: SimpleRecord) => void)
  primaryText: SimpleRenderer
  rightAvatar?: SimpleRenderer
  rightIcon?: SimpleRenderer
  secondaryText?: SimpleRenderer
  tertiaryText?: SimpleRenderer
  [key: string]: unknown
}

const LinkOrNot = ({
  classes: classesOverride,
  linkType,
  basePath,
  id,
  record,
  children,
}: {
  classes?: { link?: string; tertiary?: string }
  linkType: SimpleListProps['linkType']
  basePath: string
  id: string | number
  record: SimpleRecord
  children: React.ReactNode
}) => {
  return linkType === 'edit' || linkType === true ? (
    <Box
      component={Link}
      to={linkToRecord(basePath, id, 'edit')}
      className={classesOverride?.link}
      sx={{ textDecoration: 'none', color: 'inherit' }}
    >
      {children}
    </Box>
  ) : linkType === 'show' ? (
    <Box
      component={Link}
      to={linkToRecord(basePath, id, 'show')}
      className={classesOverride?.link}
      sx={{ textDecoration: 'none', color: 'inherit' }}
    >
      {children}
    </Box>
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
}: SimpleListProps) => {
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
