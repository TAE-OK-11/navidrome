import { useRecordContext, type RaRecord, type Identifier } from 'react-admin'

export const useRequiredRecord = <
  T extends RaRecord<Identifier> = RaRecord,
>(props?: {
  record?: T
}): T => {
  const record = useRecordContext<T>(props)
  if (!record) {
    throw new Error('Record context is missing')
  }
  return record
}
