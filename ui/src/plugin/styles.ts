import { componentStyleOverride } from '../themes/componentStyleOverride'

const slotNames = [
  'root',
  'section',
  'sectionTitle',
  'manifestBox',
  'saveButton',
  'infoGrid',
  'infoLabel',
  'pathField',
  'permissionsContainer',
  'permissionChip',
  'tooltipContent',
  'configTable',
  'configTableInput',
  'configActionIconButton',
  'usersList',
]

const classes = Object.fromEntries(
  slotNames.map((slot) => [slot, `nd-plugin-${slot}`]),
)

// Keep the existing shared `classes` contract for the plugin subcomponents,
// while the root applies all styles through one MUI sx tree.
export const usePluginShowStyles = () => classes

export const pluginShowSx = (theme) => ({
  p: 2,
  maxWidth: 900,
  ...componentStyleOverride(theme, 'NDPluginShow', 'root'),
  [`& .${classes.section}`]: {
    mb: 3,
    ...componentStyleOverride(theme, 'NDPluginShow', 'section'),
  },
  [`& .${classes.sectionTitle}`]: {
    mb: 1,
    fontWeight: 600,
    ...componentStyleOverride(theme, 'NDPluginShow', 'sectionTitle'),
  },
  [`& .${classes.manifestBox}`]: {
    backgroundColor:
      theme.palette.mode === 'dark'
        ? theme.palette.grey[900]
        : theme.palette.grey[100],
    p: 2,
    borderRadius: 1,
    fontFamily: 'monospace',
    fontSize: '0.85rem',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
    overflow: 'auto',
    maxHeight: 400,
    ...componentStyleOverride(theme, 'NDPluginShow', 'manifestBox'),
  },
  [`& .${classes.saveButton}`]: {
    mt: 2,
    ...componentStyleOverride(theme, 'NDPluginShow', 'saveButton'),
  },
  [`& .${classes.infoGrid}`]: {
    '& .MuiGrid-item': { py: 0.5 },
    ...componentStyleOverride(theme, 'NDPluginShow', 'infoGrid'),
  },
  [`& .${classes.infoLabel}`]: {
    fontWeight: 500,
    color: theme.palette.text.secondary,
    ...componentStyleOverride(theme, 'NDPluginShow', 'infoLabel'),
  },
  [`& .${classes.pathField}`]: {
    fontFamily: 'monospace',
    fontSize: '0.85rem',
    wordBreak: 'break-all',
    ...componentStyleOverride(theme, 'NDPluginShow', 'pathField'),
  },
  [`& .${classes.permissionsContainer}`]: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: 0.5,
    ...componentStyleOverride(theme, 'NDPluginShow', 'permissionsContainer'),
  },
  [`& .${classes.permissionChip}`]: {
    fontSize: '0.75rem',
    ...componentStyleOverride(theme, 'NDPluginShow', 'permissionChip'),
  },
  [`& .${classes.tooltipContent} code`]: {
    fontFamily: 'monospace',
    fontSize: '0.8em',
    backgroundColor: 'rgba(255,255,255,0.1)',
    p: '1px 4px',
    borderRadius: 0.5,
  },
  [`& .${classes.tooltipContent}`]: componentStyleOverride(
    theme,
    'NDPluginShow',
    'tooltipContent',
  ),
  [`& .${classes.configTable} .MuiTableCell-root`]: { p: 1 },
  [`& .${classes.configTable}`]: componentStyleOverride(
    theme,
    'NDPluginShow',
    'configTable',
  ),
  [`& .${classes.configTableInput}`]: {
    fontFamily: 'monospace',
    fontSize: '0.85rem',
    ...componentStyleOverride(theme, 'NDPluginShow', 'configTableInput'),
  },
  [`& .${classes.configActionIconButton}`]: {
    backgroundColor: theme.palette.action.hover,
    borderRadius: 1,
    p: 0.5,
    px: 1,
    fontWeight: 700,
    '&:hover': { backgroundColor: theme.palette.action.selected },
    ...componentStyleOverride(theme, 'NDPluginShow', 'configActionIconButton'),
  },
  [`& .${classes.usersList}`]: componentStyleOverride(
    theme,
    'NDPluginShow',
    'usersList',
  ),
})
