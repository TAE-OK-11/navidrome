// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useState } from 'react'
import { Box, Typography, Collapse } from '@mui/material'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardMedia from '@mui/material/CardMedia'
import ArtistExternalLinks from './ArtistExternalLink'
import config from '../config'
import {
  LoveButton,
  RatingField,
  ImageUploadOverlay,
  useImageLoadingState,
} from '../common'
import ImageLightbox from '../common/ImageLightbox'
import ExpandInfoDialog from '../dialogs/ExpandInfoDialog'
import AlbumInfo from '../album/AlbumInfo'
import subsonic from '../subsonic'
import { SafeHTML } from '../common/SafeHTML'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const desktopOverride = (slot) => (theme) =>
  componentStyleOverride(theme, 'NDDesktopArtistDetails', slot)

const DesktopArtistDetails = ({ artistInfo, record, biography }) => {
  const [expanded, setExpanded] = useState(false)
  const title = record.name
  const {
    imageLoading,
    imageError,
    isLightboxOpen,
    handleImageLoad,
    handleImageError,
    handleOpenLightbox,
    handleCloseLightbox,
  } = useImageLoadingState(record.id)

  return (
    <Box sx={[{ display: 'flex', p: '1em' }, desktopOverride('root')]}>
      <Card
        sx={[
          { flex: 1, p: '3%', display: 'flex', minHeight: '10rem' },
          desktopOverride('artistDetail'),
        ]}
      >
        <Card
          sx={[
            {
              maxHeight: '12rem',
              minHeight: '12rem',
              width: '12rem',
              minWidth: '12rem',
              bgcolor: 'inherit',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              boxShadow: 'none',
              position: 'relative',
            },
            desktopOverride('artistImage'),
          ]}
        >
          {artistInfo && (
            <CardMedia
              key={record.id}
              component="img"
              src={subsonic.getCoverArtUrl(record, config.uiCoverArtSize)}
              sx={[
                {
                  width: '12rem',
                  height: '12rem',
                  borderRadius: '6em',
                  cursor: imageError ? 'default' : 'pointer',
                  bgcolor: 'transparent',
                  transition: 'opacity 0.3s ease-in-out',
                  objectFit: 'cover',
                  opacity: imageLoading ? 0.5 : 1,
                },
                desktopOverride('cover'),
                imageLoading && desktopOverride('coverLoading'),
              ]}
              onClick={handleOpenLightbox}
              onLoad={handleImageLoad}
              onError={handleImageError}
              title={title}
            />
          )}
          <ImageUploadOverlay
            entityType="artist"
            entityId={record.id}
            hasUploadedImage={!!record.uploadedImage}
          />
        </Card>
        <Box
          sx={[
            { display: 'flex', flex: 1, flexDirection: 'column' },
            desktopOverride('details'),
          ]}
        >
          <CardContent sx={[{ flex: '1 0 auto' }, desktopOverride('content')]}>
            <Typography
              component="h5"
              variant="h5"
              sx={[{ wordBreak: 'break-word' }, desktopOverride('artistName')]}
            >
              {title}
              <LoveButton
                sx={[{ top: -0.2, left: 0.5 }, desktopOverride('loveButton')]}
                record={record}
                resource={'artist'}
                size={'default'}
                aria-label="artist context menu"
                color="primary"
              />
            </Typography>
            {config.enableStarRating && (
              <div>
                <RatingField
                  record={record}
                  resource={'artist'}
                  size={'small'}
                  sx={[{ mt: '5px' }, desktopOverride('rating')]}
                />
              </div>
            )}
            <Collapse
              collapsedSize={'4.5em'}
              in={expanded}
              timeout={'auto'}
              sx={[
                {
                  display: 'inline-block',
                  mt: '1em',
                  float: 'left',
                  wordBreak: 'break-word',
                  cursor: 'pointer',
                  minHeight: '4.5em',
                },
                desktopOverride('biography'),
              ]}
            >
              <Typography
                variant={'body1'}
                onClick={() => setExpanded(!expanded)}
              >
                <span>
                  <SafeHTML>{biography}</SafeHTML>
                </span>
              </Typography>
            </Collapse>
          </CardContent>
          <Typography
            component={'div'}
            sx={[{ ml: '0.9em' }, desktopOverride('button')]}
          >
            {config.enableExternalServices && (
              <ArtistExternalLinks artistInfo={artistInfo} record={record} />
            )}
          </Typography>
        </Box>
        <ImageLightbox
          open={isLightboxOpen && !imageError}
          imageUrl={subsonic.getCoverArtUrl(record)}
          title={record.name}
          onClose={handleCloseLightbox}
        />
      </Card>
      <ExpandInfoDialog content={<AlbumInfo />} />
    </Box>
  )
}

export default DesktopArtistDetails
