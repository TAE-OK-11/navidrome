// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import PropTypes from 'prop-types'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemText from '@mui/material/ListItemText'
import Box from '@mui/material/Box'
import { sanitizeListRestProps, useListContext } from 'react-admin'
import { DurationField, SongContextMenu, RatingField } from './index'
import { setTrack } from '../actions'
import { useDispatch } from 'react-redux'
import config from '../config'

export const SongSimpleList = ({
  className,
  classes: classesOverride,
  hasBulkActions = false,
  ...rest
}) => {
  const dispatch = useDispatch()
  const { data = [], isPending, total = 0 } = useListContext()
  return (
    (isPending || total > 0) && (
      <List className={className} {...sanitizeListRestProps(rest)}>
        {data.map(
          (record) =>
            record && (
              <span key={record.id} onClick={() => dispatch(setTrack(record))}>
                <ListItemButton
                  className={classesOverride?.listItem}
                  sx={{ p: '10px' }}
                >
                  <ListItemText
                    primary={
                      <Box
                        className={classesOverride?.title}
                        sx={{ pr: '10px', width: '80%' }}
                      >
                        {record.title}
                      </Box>
                    }
                    secondary={
                      <>
                        <Box
                          component="span"
                          className={classesOverride?.secondary}
                          sx={{
                            mt: '-3px',
                            width: '96%',
                            display: 'flex',
                            alignItems: 'flex-start',
                            justifyContent: 'space-between',
                          }}
                        >
                          <Box
                            component="span"
                            className={classesOverride?.artist}
                            sx={{ pr: '30px' }}
                          >
                            {record.artist}
                          </Box>
                          <Box
                            component="span"
                            className={classesOverride?.timeStamp}
                            sx={{
                              float: 'right',
                              color: '#fff',
                              fontWeight: 200,
                              opacity: 0.6,
                              fontSize: 12,
                              p: '2px',
                            }}
                          >
                            <DurationField
                              record={record}
                              source={'duration'}
                            />
                          </Box>
                        </Box>
                        {config.enableStarRating && (
                          <RatingField
                            record={record}
                            source={'rating'}
                            resource={'song'}
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
                      <SongContextMenu record={record} visible={true} />
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

SongSimpleList.propTypes = {
  basePath: PropTypes.string,
  className: PropTypes.string,
  classes: PropTypes.object,
  data: PropTypes.object,
  hasBulkActions: PropTypes.bool.isRequired,
  ids: PropTypes.array,
  onToggleItem: PropTypes.func,
  selectedIds: PropTypes.arrayOf(PropTypes.any).isRequired,
}
