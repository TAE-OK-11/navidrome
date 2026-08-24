// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import {
  Card,
  CardContent,
  CardMedia,
  Typography,
  useMediaQuery,
  Box,
} from '@mui/material'
import { useTranslate } from 'react-admin'
import Lightbox from 'react-image-lightbox'
import 'react-image-lightbox/style.css'
import {
  CollapsibleComment,
  DurationField,
  ImageUploadOverlay,
  LoveButton,
  SizeField,
  isWritable,
  OverflowTooltip,
  useImageLoadingState,
} from '../common'
import config from '../config'
import subsonic from '../subsonic'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const playlistOverride = (slot) => (theme) =>
  componentStyleOverride(theme, 'NDPlaylistDetails', slot)

const PlaylistDetails = (props) => {
  const { record = {} } = props
  const translate = useTranslate()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('lg'))
  const {
    imageLoading,
    imageError,
    isLightboxOpen,
    handleImageLoad,
    handleImageError,
    handleOpenLightbox,
    handleCloseLightbox,
  } = useImageLoadingState(record.id)

  const imageUrl = subsonic.getCoverArtUrl(record, config.uiCoverArtSize, true)
  const fullImageUrl = subsonic.getCoverArtUrl(record)

  return (
    <Card
      sx={[
        (theme) => ({
          [theme.breakpoints.down('sm')]: { p: '0.7em', minWidth: '20em' },
          [theme.breakpoints.up('sm')]: { p: '1em', minWidth: '32em' },
        }),
        playlistOverride('root'),
      ]}
    >
      <Box sx={[{ display: 'flex' }, playlistOverride('cardContents')]}>
        <Box
          sx={[
            (theme) => ({
              [theme.breakpoints.down('sm')]: {
                height: '8em',
                width: '8em',
                minWidth: '8em',
              },
              [theme.breakpoints.up('sm')]: {
                height: '10em',
                width: '10em',
                minWidth: '10em',
              },
              [theme.breakpoints.up('lg')]: {
                height: '15em',
                width: '15em',
                minWidth: '15em',
              },
              bgcolor: 'transparent',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              position: 'relative',
            }),
            playlistOverride('coverParent'),
          ]}
        >
          <CardMedia
            key={record.id} // Force re-render when playlist changes
            component={'img'}
            src={imageUrl}
            width="400"
            height="400"
            sx={[
              {
                objectFit: 'contain',
                cursor: imageError ? 'default' : 'pointer',
                display: 'block',
                width: '100%',
                height: '100%',
                bgcolor: 'transparent',
                transition: 'opacity 0.3s ease-in-out',
                opacity: imageLoading ? 0.5 : 1,
              },
              playlistOverride('cover'),
              imageLoading && playlistOverride('coverLoading'),
            ]}
            onClick={handleOpenLightbox}
            onLoad={handleImageLoad}
            onError={handleImageError}
            title={record.name}
          />
          {isWritable(record.ownerId) && (
            <ImageUploadOverlay
              entityType="playlist"
              entityId={record.id}
              hasUploadedImage={!!record.uploadedImage}
            />
          )}
        </Box>
        <Box
          sx={[
            { display: 'flex', flexDirection: 'column' },
            playlistOverride('details'),
          ]}
        >
          <CardContent sx={[{ flex: '2 0 auto' }, playlistOverride('content')]}>
            <Box
              sx={[
                { display: 'flex', alignItems: 'center' },
                playlistOverride('titleRow'),
              ]}
            >
              <OverflowTooltip title={record.name || ''}>
                <Typography
                  variant={isDesktop ? 'h5' : 'h6'}
                  sx={[
                    {
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      wordBreak: 'break-word',
                      minWidth: 0,
                    },
                    playlistOverride('title'),
                  ]}
                >
                  {record.name || translate('ra.page.loading')}
                </Typography>
              </OverflowTooltip>
              <LoveButton
                sx={[
                  { ml: 0.5, flexShrink: 0 },
                  playlistOverride('loveButton'),
                ]}
                record={record}
                resource={'playlist'}
                size={isDesktop ? 'default' : 'small'}
                aria-label="love"
                color="primary"
              />
            </Box>
            <Typography
              component="p"
              sx={[{ mt: '1em', mb: '0.5em' }, playlistOverride('stats')]}
            >
              {record.songCount ? (
                <span>
                  {record.songCount}{' '}
                  {translate('resources.song.name', {
                    smart_count: record.songCount,
                  })}
                  {' · '}
                  <DurationField record={record} source={'duration'} />
                  {' · '}
                  <SizeField record={record} source={'size'} />
                </span>
              ) : (
                <span>&nbsp;</span>
              )}
            </Typography>
            <CollapsibleComment record={record} />
          </CardContent>
        </Box>
      </Box>
      {isLightboxOpen && !imageError && (
        <Lightbox
          imagePadding={50}
          animationDuration={200}
          imageTitle={record.name}
          mainSrc={fullImageUrl}
          onCloseRequest={handleCloseLightbox}
        />
      )}
    </Card>
  )
}

export default PlaylistDetails
