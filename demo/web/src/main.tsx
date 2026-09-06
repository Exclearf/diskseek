import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { CssBaseline, InitColorSchemeScript, ThemeProvider, createTheme } from '@mui/material'
import App from './App'

const theme = createTheme({
  cssVariables: { colorSchemeSelector: 'class' },
  colorSchemes: {
    light: {
      palette: {
        primary: { main: '#4c637d' },
        background: { default: '#f7f8fa', paper: '#ffffff' },
        text: { primary: '#1f2328', secondary: '#656d76' },
      },
    },
    dark: {
      palette: {
        primary: { main: '#a9bdd3' },
        background: { default: '#17191c', paper: '#212428' },
        text: { primary: '#f0f2f4', secondary: '#b2b8c0' },
      },
    },
  },
  typography: {
    fontFamily: 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    button: {
      textTransform: 'none',
    },
  },
  components: {
    MuiButton: { defaultProps: { disableElevation: true } },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <InitColorSchemeScript attribute="class" modeStorageKey="diskseek-mode" />
    <ThemeProvider theme={theme} defaultMode="system" modeStorageKey="diskseek-mode" disableTransitionOnChange>
      <CssBaseline />
      <App />
    </ThemeProvider>
  </StrictMode>,
)
