// Заголовок вкладки и meta description для публичных страниц SPA.
// Базовые значения лежат в index.html (для соцсетей и поисковиков), а при
// переходах страница уточняет их под свой контент.
import { useEffect } from 'react';

export const DEFAULT_TITLE = 'skillex.space — IT-курсы для начинающих';
export const DEFAULT_DESCRIPTION =
  'Онлайн-курсы для начинающих: короткие видеоуроки, конспекты и тесты с автоматической проверкой. Учитесь в своём темпе и получайте сертификат о прохождении курса.';

function setMetaDescription(content: string): void {
  let meta = document.head.querySelector<HTMLMetaElement>('meta[name="description"]');
  if (!meta) {
    meta = document.createElement('meta');
    meta.name = 'description';
    document.head.appendChild(meta);
  }
  meta.content = content;
}

/** Меняет title и description при монтировании/обновлении страницы. */
export function useSeo(title: string = DEFAULT_TITLE, description: string = DEFAULT_DESCRIPTION): void {
  useEffect(() => {
    document.title = title;
    setMetaDescription(description);
  }, [title, description]);
}
