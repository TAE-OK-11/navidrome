// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
export const removeAlbumCommentsFromSongs = ({ album, data }) => {
  if (album?.comment && data) {
    Object.values(data).forEach((song) => {
      song.comment = ''
    })
  }
}
