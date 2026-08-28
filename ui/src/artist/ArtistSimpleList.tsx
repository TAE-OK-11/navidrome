// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemAvatar from '@mui/material/ListItemAvatar'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemText from '@mui/material/ListItemText'
import Box from '@mui/material/Box'
import { sanitizeListRestProps, useListContext } from 'react-admin'
import { ArtistContextMenu, CoverArtAvatar, RatingField } from '../common'
import config from '../config'

const ArtistSimpleList = ({
  linkType,
  className,
  classes: classesOverride,
  hasBulkActions = false,
  ...rest
}) => {
  const { data = [], isPending, total = 0 } = useListContext()
  return (
    (isPending || total > 0) && (
      <List className={className} {...sanitizeListRestProps(rest)}>
        {data.map(
          (record) =>
            record && (
              <span key={record.id} onClick={() => linkType(record.id)}>
                <ListItemButton
                  className={classesOverride?.listItem}
                  sx={{ p: '10px' }}
                >
                  <ListItemAvatar>
                    <CoverArtAvatar record={record} />
                  </ListItemAvatar>
                  <ListItemText
                    sx={{ marginLeft: '8px' }}
                    primary={
                      <>
                        <Box
                          className={classesOverride?.title}
                          sx={{ pr: '10px', width: '80%' }}
                        >
                          {record.name}
                        </Box>
                        {config.enableStarRating && (
                          <RatingField
                            record={record}
                            source={'rating'}
                            resource={'artist'}
                            size={'small'}
                          />
                        )}
                      </>
                    }
                  />
                  <ListItemSecondaryAction
                    className={classesOverride?.rightIcon}
                    sx={{ top: 26 }}
                  >
                    <ListItemIcon>
                      <ArtistContextMenu record={record} />
                    </ListItemIcon>
                  </ListItemSecondaryAction>
                </ListItemButton>
              </span>
            ),
        )}
      </List>
    )
  )
}

export default ArtistSimpleList
