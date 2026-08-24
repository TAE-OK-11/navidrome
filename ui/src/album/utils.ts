type CommentRecord = { comment?: string }

type RemoveAlbumCommentsParams = {
  album?: CommentRecord
  data?: Record<string, CommentRecord> | CommentRecord[]
}

export const removeAlbumCommentsFromSongs = ({
  album,
  data,
}: RemoveAlbumCommentsParams) => {
  if (album?.comment && data) {
    Object.values(data).forEach((song) => {
      song.comment = ''
    })
  }
}
