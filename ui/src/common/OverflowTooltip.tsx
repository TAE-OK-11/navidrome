import React from 'react'
import { Tooltip } from '@mui/material'
import { alpha } from '@mui/material/styles'
import { grey } from '@mui/material/colors'
import type { Theme } from '@mui/material/styles'
import type { TooltipProps } from '@mui/material/Tooltip'

const tooltipSx = (theme: Theme) => ({
  backgroundColor:
    theme.palette.mode === 'dark'
      ? alpha(grey[700], 0.92)
      : alpha(grey[300], 0.92),
  color:
    theme.palette.mode === 'dark'
      ? theme.palette.common.white
      : theme.palette.common.black,
  borderRadius: theme.shape.borderRadius,
  ...theme.typography.body2,
  p: theme.spacing(0.5, 1),
  maxWidth: 300,
})

const transitionProps = { timeout: 0 }

type RefChildProps = { ref?: React.Ref<HTMLElement> }

type OverflowTooltipProps = {
  children: React.ReactElement<RefChildProps>
  title: string
  placement?: TooltipProps['placement']
}

export const OverflowTooltip = ({
  children,
  title,
  placement = 'bottom-start',
}: OverflowTooltipProps) => {
  const textRef = React.useRef<HTMLElement | null>(null)
  const [isOverflowing, setIsOverflowing] = React.useState(false)

  React.useLayoutEffect(() => {
    const el = textRef.current
    if (!el) return

    const checkOverflow = () => {
      setIsOverflowing(el.scrollWidth > el.clientWidth)
    }

    const resizeObserver = new ResizeObserver(checkOverflow)
    resizeObserver.observe(el)

    checkOverflow()

    return () => resizeObserver.disconnect()
  }, [])

  const mergedRef = React.useCallback(
    (el: HTMLElement | null) => {
      textRef.current = el

      const { ref } = children.props
      if (typeof ref === 'function') {
        ref(el)
      } else if (ref && typeof ref === 'object') {
        ref.current = el
      }
    },
    [children],
  )

  return (
    <Tooltip
      title={title}
      disableHoverListener={!isOverflowing}
      disableTouchListener
      placement={placement}
      slotProps={{ transition: transitionProps, tooltip: { sx: tooltipSx } }}
    >
      {React.cloneElement(children, { ref: mergedRef })}
    </Tooltip>
  )
}
