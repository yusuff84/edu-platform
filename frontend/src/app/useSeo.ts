// Заголовок вкладки и meta description для публичных страниц SPA.
// Базовые значения лежат в index.html (для соцсетей и поисковиков), а при
// переходах страница уточняет их под свой контент.
import { useEffect } from 'react';

export const DEFAULT_TITLE = 'Edu Platform — IT-курсы для начинающих';
export const DEFAULT_DESCRIPTION =
  'Маркетплейс IT-курсов для начинающих: Python, веб, данные и дизайн. Практика, живые разборы, квизы и кодовые задачи. Оплата в рублях, возврат в течение 7 дней.';

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
