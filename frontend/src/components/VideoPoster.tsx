import { useState } from 'react';
import { videoPosterUrl } from '../app/media';

/**
 * Кадр из видео урока для карточки. Если превью нет (внешняя ссылка или
 * кадр ещё не сгенерирован), ничего не рендерит — остаётся заглушка-градиент.
 */
export function VideoPoster({ videoUrl, className }: { videoUrl?: string | null; className?: string }): JSX.Element | null {
  const src = videoPosterUrl(videoUrl);
  const [failed, setFailed] = useState(false);
  if (!src || failed) return null;
  return (
    <img
      className={className ?? 'video-poster'}
      src={src}
      alt=""
      loading="lazy"
      decoding="async"
      onError={() => setFailed(true)}
    />
  );
}
