import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Библиотеки, общие для всех страниц (React, роутер, react-query), — в отдельный
// vendor-чанк: он почти не меняется между релизами и остаётся в кеше браузера,
// когда меняется код приложения. Остальные зависимости (react-hook-form, qrcode)
// Rollup раскладывает по ленивым чанкам страниц, которые их используют.
const VENDOR_RE =
  /[\\/]node_modules[\\/](react|react-dom|scheduler|react-router|react-router-dom|@tanstack[\\/](react-query|query-core)|cookie|set-cookie-parser)[\\/]/;

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      output: {
        // SSR-сборка контрактных тестов (npm test) не затрагивается: там
        // node_modules внешние и в чанки не попадают.
        manualChunks: (id: string) => (VENDOR_RE.test(id) ? 'vendor' : undefined),
      },
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY ?? 'http://localhost:8082',
        changeOrigin: true,
      },
      // T54+T57: обложки курсов (/uploads/covers/...) отдаёт api — в dev-режиме
      // тоже нужен прокси, иначе <img> получает index.html SPA.
      '/uploads': {
        target: process.env.VITE_API_PROXY ?? 'http://localhost:8082',
        changeOrigin: true,
      },
      // Mock-страница оплаты (/pay/mock/{id}) — HTML backend, не SPA-роут.
      // Без прокси dev-сервер отдаёт index.html и покупка «теряется».
      '/pay/mock': {
        target: process.env.VITE_API_PROXY ?? 'http://localhost:8082',
        changeOrigin: true,
      },
    },
  },
  preview: {
    port: 3000,
  },
});
