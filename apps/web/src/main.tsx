import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import "@fontsource/poppins/600.css";
import "@fontsource/open-sans/400.css";
import "@fontsource/open-sans/600.css";

import { GlobalErrorBoundary } from './components/GlobalErrorBoundary.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <GlobalErrorBoundary>
      <App />
    </GlobalErrorBoundary>
  </StrictMode>,
)
