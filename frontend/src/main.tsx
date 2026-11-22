import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from 'react-router-dom';
import { router } from './app/router';
import { reloadForNewVersion } from './app/chunkReload';
import './styles.css';
import './styles/mobile.css';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
});

// Vite сообщает, что не смог подгрузить часть приложения (обычно — после выкладки новой версии):
// перезагружаемся на новую версию вместо экрана ошибки.
window.addEventListener('vite:preloadError', (event) => {
  if (reloadForNewVersion()) event.preventDefault();
});

const rootEl = document.getElementById('root');
if (!rootEl) throw new Error('Корневой элемент #root не найден');

ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </React.StrictMode>,
);
