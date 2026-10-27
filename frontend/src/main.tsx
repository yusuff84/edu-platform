import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from 'react-router-dom';
import { router } from './app/router';
import './styles.css';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
});

// Стартовый сплэш живёт в index.html, чтобы появиться до JS. Держим его
// минимум ~0.9 с от начала навигации (анимация успевает прочитаться, но
// сплэш не задерживает приложение) и убираем после первого кадра React.
// При reduce-motion — статичный логотип на более короткий срок.
const SPLASH_FADE_MS = 520;
const SPLASH_MIN_MS = 900;
const SPLASH_MIN_REDUCED_MS = 350;

function dismissSplash(): void {
  const splash = document.getElementById('splash');
  if (!splash) return;

  splash.classList.add('splash--hidden');
  splash.setAttribute('aria-hidden', 'true');
  window.setTimeout(() => splash.remove(), SPLASH_FADE_MS);
}

const rootEl = document.getElementById('root');
if (!rootEl) throw new Error('Корневой элемент #root не найден');

ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </React.StrictMode>,
);

const reduceMotion =
  window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
const minSplashMs = reduceMotion ? SPLASH_MIN_REDUCED_MS : SPLASH_MIN_MS;

// Два rAF гарантируют, что приложение уже отрисовано под оверлеем, а
// performance.now() учитывает время, пока грузился сам бандл.
window.requestAnimationFrame(() => {
  window.requestAnimationFrame(() => {
    const delay = Math.max(0, minSplashMs - performance.now());
    window.setTimeout(dismissSplash, delay);
  });
});
