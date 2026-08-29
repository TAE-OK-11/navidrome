import type { ReactNode } from 'react'

/* intersperse: Return an array with the separator interspersed between
 * each element of the input array.
 *
 * > _([1,2,3]).intersperse(0)
 * [1,0,2,0,3]
 *
 * From: https://stackoverflow.com/a/23619085
 */
export const intersperse = <T extends ReactNode>(
  arr: T[],
  sep: ReactNode,
): ReactNode[] => {
  if (arr.length === 0) {
    return []
  }

  return arr.slice(1).reduce<ReactNode[]>(
    function (xs, x) {
      return xs.concat([sep, x])
    },
    [arr[0]],
  )
}
