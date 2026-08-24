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
import Lightbox from 'react-image-lightbox'
import ExpandInfoDialog from '../dialogs/ExpandInfoDialog'
import AlbumInfo from '../album/AlbumInfo'
import subsonic from '../subsonic'
import { SafeHTML } from '../common/SafeHTML'

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
    <Box sx={{ display: 'flex', p: '1em' }}>
      <Card sx={{ flex: 1, p: '3%', display: 'flex', minHeight: '10rem' }}>
        <Card
          sx={{
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
          }}
        >
          {artistInfo && (
            <CardMedia
              key={record.id}
              component="img"
              src={subsonic.getCoverArtUrl(record, config.uiCoverArtSize)}
              sx={{
                width: '12rem',
                height: '12rem',
                borderRadius: '6em',
                cursor: imageError ? 'default' : 'pointer',
                bgcolor: 'transparent',
                transition: 'opacity 0.3s ease-in-out',
                objectFit: 'cover',
                opacity: imageLoading ? 0.5 : 1,
              }}
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
        <Box sx={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
          <CardContent sx={{ flex: '1 0 auto' }}>
            <Typography
              component="h5"
              variant="h5"
              sx={{ wordBreak: 'break-word' }}
            >
              {title}
              <LoveButton
                sx={{ top: -0.2, left: 0.5 }}
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
                  sx={{ mt: '5px' }}
                />
              </div>
            )}
            <Collapse
              collapsedSize={'4.5em'}
              in={expanded}
              timeout={'auto'}
              sx={{
                display: 'inline-block',
                mt: '1em',
                float: 'left',
                wordBreak: 'break-word',
                cursor: 'pointer',
                minHeight: '4.5em',
              }}
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
          <Typography component={'div'} sx={{ ml: '0.9em' }}>
            {config.enableExternalServices && (
              <ArtistExternalLinks artistInfo={artistInfo} record={record} />
            )}
          </Typography>
        </Box>
        {isLightboxOpen && !imageError && (
          <Lightbox
            imagePadding={50}
            animationDuration={200}
            imageTitle={record.name}
            mainSrc={subsonic.getCoverArtUrl(record)}
            onCloseRequest={handleCloseLightbox}
          />
        )}
      </Card>
      <ExpandInfoDialog content={<AlbumInfo />} />
    </Box>
  )
}

export default DesktopArtistDetails
