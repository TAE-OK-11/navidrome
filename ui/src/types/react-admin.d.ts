import 'react-admin'

declare module 'react-admin' {
  interface ReferenceArrayInputProps {
    filterToQuery?: (searchText: string) => Record<string, unknown>
  }

  interface ReferenceManyFieldProps {
    addLabel?: boolean
  }
}

declare module 'ra-core' {
  interface UpdateParams<RecordType = unknown> {
    filter?: Record<string, unknown>
  }
}
