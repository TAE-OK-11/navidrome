export const openInNewTab = (url: string) => {
  const win = window.open(url, '_blank')
  win?.focus()
  return win
}
