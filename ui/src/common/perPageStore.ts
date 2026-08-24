export const defaultRowsPerPageOptions = [15, 25, 50]

const key = (resource: string) => `perPage.${resource}`

export const getStoredPerPage = (
  resource: string,
  options: readonly number[],
  fallback = options[0],
) => {
  const stored = Number.parseInt(localStorage.getItem(key(resource)) ?? '', 10)
  return options.includes(stored) ? stored : fallback
}

export const setStoredPerPage = (resource: string, perPage: number) =>
  localStorage.setItem(key(resource), String(perPage))
