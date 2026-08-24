// User form validation utilities
type UserFormValues = {
  isAdmin?: boolean
  libraryIds?: string[]
  libraries?: Array<{ id?: string }>
}

type Translate = (key: string) => string

export const validateUserForm = (
  values: UserFormValues,
  translate: Translate,
) => {
  const errors: { libraryIds?: string } = {}

  // Only require library selection for non-admin users
  if (!values.isAdmin) {
    // Check both libraryIds (array of IDs) and libraries (array of objects)
    const hasLibraryIds = values.libraryIds && values.libraryIds.length > 0
    const hasLibraries = values.libraries && values.libraries.length > 0

    if (!hasLibraryIds && !hasLibraries) {
      errors.libraryIds = translate(
        'resources.user.validation.librariesRequired',
      )
    }
  }

  return errors
}
