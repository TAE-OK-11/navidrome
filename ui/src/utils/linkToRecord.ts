export const linkToRecord = (basePath, id, linkType) => {
  const recordPath = `${basePath}/${encodeURIComponent(id)}`
  return linkType === 'show' ? `${recordPath}/show` : recordPath
}
