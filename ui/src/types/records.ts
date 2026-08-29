import type { Identifier, RaRecord } from 'react-admin'

export type NavidromeRecord = RaRecord<Identifier> & Record<string, unknown>

export type ArtistParticipant = {
  id: Identifier
  name: string
  subroles?: string[]
}

export type ParticipantsRecord = NavidromeRecord & {
  participants?: Record<string, ArtistParticipant[]>
  albumArtist?: string
  displayAlbumArtist?: string
  tags?: Record<string, string[] | string>
}

export type AlbumRecord = ParticipantsRecord & {
  id: Identifier
  name?: string
  notes?: string
  genres?: Array<{ id: Identifier; name: string }>
  genre?: string
  minYear?: number | string
  maxYear?: number | string
  minOriginalYear?: number | string
  maxOriginalYear?: number | string
  year?: number | string
  date?: string
  originalDate?: string
  releaseDate?: string
  songCount?: number
  duration?: number
  size?: number
  missing?: boolean
  starred?: boolean
  comment?: string
  mbzAlbumId?: string
  coverArtId?: Identifier
  updatedAt?: string
  libraryName?: string
  catalogNum?: string
  compilation?: boolean
}

export type SongRecord = ParticipantsRecord & {
  id: Identifier
  title?: string
  artist?: string
  album?: string
  albumId?: Identifier
  duration?: number
  updatedAt?: string
  trackNumber?: number
  year?: number | string
  missing?: boolean
  mediaFileId?: Identifier
}

export type ArtistRecord = NavidromeRecord & {
  id: Identifier
  name?: string
  biography?: string
  missing?: boolean
  uploadedImage?: boolean
  stats?: Record<
    string,
    { albumCount?: number; songCount?: number; size?: number }
  >
}

export type PlaylistSelection = {
  id?: Identifier
  name: string
  distinctIds?: Identifier[]
}

export type PlaylistRecord = NavidromeRecord & {
  id: Identifier
  name?: string
  comment?: string
  songCount?: number
  duration?: number
  size?: number
  public?: boolean
  sync?: boolean
  path?: string
  ownerId?: Identifier
  ownerName?: string
  uploadedImage?: boolean
  starred?: boolean
}

export type UserFormValues = {
  isAdmin?: boolean
  libraryIds?: string[]
  libraries?: Array<{ id?: string; name?: string }>
  [key: string]: unknown
}
