import React from 'react'
import PropTypes from 'prop-types'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemAvatar from '@mui/material/ListItemAvatar'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import ListItemText from '@mui/material/ListItemText'
import makeStyles from '../themes/makeStyles'
import { sanitizeListRestProps, useListContext } from 'react-admin'
import { ArtistContextMenu, CoverArtAvatar, RatingField } from '../common'
import config from '../config'

const useStyles = makeStyles(
  {
    listItem: {
      padding: '10px',
    },
    title: {
      paddingRight: '10px',
      width: '80%',
    },
    rightIcon: {
      top: '26px',
    },
  },
  { name: 'RaArtistSimpleList' },
)

const ArtistSimpleList = ({
  linkType,
  className,
  classes: classesOverride,
  hasBulkActions = false,
  ...rest
}) => {
  const classes = useStyles({ classes: classesOverride })
  const { data = [], isPending, total = 0 } = useListContext()
  return (
    (isPending || total > 0) && (
      <List className={className} {...sanitizeListRestProps(rest)}>
        {data.map(
          (record) =>
            record && (
              <span key={record.id} onClick={() => linkType(record.id)}>
                <ListItemButton className={classes.listItem}>
                  <ListItemAvatar>
                    <CoverArtAvatar record={record} />
                  </ListItemAvatar>
                  <ListItemText
                    style={{ marginLeft: '8px' }}
                    primary={
                      <>
                        <div className={classes.title}>{record.name}</div>
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
                  <ListItemSecondaryAction className={classes.rightIcon}>
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

ArtistSimpleList.propTypes = {
  className: PropTypes.string,
  classes: PropTypes.object,
  data: PropTypes.object,
  hasBulkActions: PropTypes.bool.isRequired,
  ids: PropTypes.array,
  selectedIds: PropTypes.arrayOf(PropTypes.any).isRequired,
}

export default ArtistSimpleList
