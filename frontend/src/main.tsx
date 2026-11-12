import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@/styles/reset.css';
import '@/styles/tokens.css';
import '@/styles/base.css';
import '@/styles/layouts.css';
import { App } from './App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

