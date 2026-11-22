// После выкладки новой версии старые файлы сборки (assets/*-<хеш>.js) удаляются.
// Вкладка, открытая до обновления, при переходе на ещё не загруженную страницу
// просит старый файл и получает ошибку «Failed to fetch dynamically imported module».
// Лечится перезагрузкой: браузер возьмёт новый index.html и новые файлы.

const KEY = 'skx-chunk-reload-at';

/** Ошибка загрузки части приложения (динамический import / preload CSS). */
export function isChunkLoadError(error: unknown): boolean {
  const msg = error instanceof Error ? `${error.name} ${error.message}` : String(error ?? '');
  return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|Unable to preload CSS|ChunkLoadError|Loading chunk [\w-]+ failed/i.test(msg);
}

/** Перезагружает страницу, если не делали этого последние 30 секунд (защита от цикла). */
export function reloadForNewVersion(): boolean {
  try {
    const last = Number(window.sessionStorage.getItem(KEY) || 0);
    if (Date.now() - last < 30_000) return false;
    window.sessionStorage.setItem(KEY, String(Date.now()));
  } catch {
    /* sessionStorage недоступен — перезагружаем без защиты, один раз за загрузку */
  }
  window.location.reload();
  return true;
}
