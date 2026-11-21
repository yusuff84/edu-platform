// Превью-кадр видео урока. Для видео из /uploads сервер хранит кадр рядом
// с файлом: <имя>.mp4 → <имя>.poster.jpg (scripts/generate-video-posters.sh).
// Для внешних ссылок превью нет — компоненты показывают заглушку.
export function videoPosterUrl(videoUrl: string | null | undefined): string | null {
  if (!videoUrl || !videoUrl.startsWith('/uploads/')) return null;
  const clean = videoUrl.split(/[?#]/)[0];
  if (!/\.(mp4|webm|mov)$/i.test(clean)) return null;
  return clean.replace(/\.(mp4|webm|mov)$/i, '.poster.jpg');
}
