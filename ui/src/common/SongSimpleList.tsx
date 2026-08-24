// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import PropTypes from 'prop-types'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemText from '@mui/material/ListItemText'
import makeStyles from '../themes/makeStyles'
import { sanitizeListRestProps, useListContext } from 'react-admin'
import { DurationField, SongContextMenu, RatingField } from './index'
import { setTrack } from '../actions'
import { useDispatch } from 'react-redux'
import config from '../config'

const useStyles = makeStyles(
  {
    link: {
      textDecoration: 'none',
      color: 'inherit',
    },
    listItem: {
      padding: '10px',
    },
    title: {
      paddingRight: '10px',
      width: '80%',
    },
    secondary: {
      marginTop: '-3px',
      width: '96%',
      display: 'flex',
      alignItems: 'flex-start',
      justifyContent: 'space-between',
    },
    artist: {
      paddingRight: '30px',
    },
    timeStamp: {
      float: 'right',
      color: '#fff',
      fontWeight: '200',
      opacity: 0.6,
      fontSize: '12px',
      padding: '2px',
    },
    rightIcon: {
      top: '26px',
    },
  },
  { name: 'RaSongSimpleList' },
)

export const SongSimpleList = ({
  className,
  classes: classesOverride,
  hasBulkActions = false,
  ...rest
}) => {
  const dispatch = useDispatch()
  const classes = useStyles({ classes: classesOverride })
  const { data = [], isPending, total = 0 } = useListContext()
  return (
    (isPending || total > 0) && (
      <List className={className} {...sanitizeListRestProps(rest)}>
        {data.map(
          (record) =>
            record && (
              <span key={record.id} onClick={() => dispatch(setTrack(record))}>
                <ListItemButton className={classes.listItem}>
                  <ListItemText
                    primary={
                      <div className={classes.title}>{record.title}</div>
                    }
                    secondary={
                      <>
                        <span className={classes.secondary}>
                          <span className={classes.artist}>
                            {record.artist}
                          </span>
                          <span className={classes.timeStamp}>
                            <DurationField
                              record={record}
                              source={'duration'}
                            />
                          </span>
                        </span>
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
                  <ListItemSecondaryAction className={classes.rightIcon}>
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
