// Download media URLs observed in the rendered browser DOM and build a local reader.
// This uses ordinary media file URLs, never the Stepik API.
const fs = require('node:fs/promises');
const { createWriteStream } = require('node:fs');
const path = require('node:path');
const { Readable } = require('node:stream');
const { pipeline } = require('node:stream/promises');
const { createHash } = require('node:crypto');
const root = process.env.STEPIK_EXPORT_DIR || path.resolve(__dirname, '../.local/stepik-146168');
const esc = s => String(s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);

async function main() {
  const manifest = JSON.parse(await fs.readFile(path.join(root, 'manifest.json'), 'utf8'));
  const exported = manifest.modules.flatMap(m => m.lessons.flatMap(l => l.steps || [])).length;
  if (exported !== manifest.expectedStepCount || manifest.errors?.length) {
    throw new Error('Finish the browser export first: ' + exported + '/' + manifest.expectedStepCount + ' steps.');
  }
  const assets = [];
  const errors = [];
  const steps = [];
  const byUrl = new Map();
  for (const module of manifest.modules) for (const lesson of module.lessons) for (const ref of lesson.steps || []) {
    const data = JSON.parse(await fs.readFile(path.join(root, ref.file), 'utf8'));
    steps.push({ module, lesson, ref, data });
    const videos = [...data.videos].sort((a, b) => Number(b.match(/\/(\d+)\/[a-f\d]+\.mp4/)?.[1] || 0) - Number(a.match(/\/(\d+)\/[a-f\d]+\.mp4/)?.[1] || 0));
    const urls = [...(videos[0] ? [{ url: videos[0], kind: 'video' }] : []), ...data.images.map(i => ({ url: i.url, kind: 'image' })), ...data.posters.map(url => ({ url, kind: 'image' }))];
    for (const item of urls) {
      if (!item.url || byUrl.has(item.url)) continue;
      const parsed = new URL(item.url);
      if (parsed.protocol !== 'https:') continue;
      const ext = item.kind === 'video' ? '.mp4' : (path.extname(parsed.pathname).match(/^\.[a-z0-9]{1,5}$/i)?.[0] || '.image');
      const file = 'assets/' + createHash('sha256').update(item.url).digest('hex').slice(0, 20) + ext;
      const asset = { ...item, file };
      assets.push(asset);
      byUrl.set(item.url, asset);
    }
  }
  await fs.mkdir(path.join(root, 'assets'), { recursive: true });
  async function download(asset) {
    const target = path.join(root, asset.file);
    try {
      const existing = await fs.stat(target).catch(() => null);
      if (existing?.size > 0) {
        asset.bytes = existing.size;
        console.log('CACHED_MEDIA', asset.file, existing.size);
        return;
      }
      let lastError;
      for (let retry = 0; retry < 6; retry++) {
        const bytesBefore = (await fs.stat(target + '.part').catch(() => null))?.size || 0;
        try {
          let offset = asset.kind === 'video' ? (await fs.stat(target + '.part').catch(() => null))?.size || 0 : 0;
          let total;
          let mime;
          do {
            const headers = { Referer: 'https://stepik.org/', 'User-Agent': 'Mozilla/5.0' };
            if (asset.kind === 'video') headers.Range = 'bytes=' + offset + '-' + (offset + 1024 * 1024 - 1);
            const response = await fetch(asset.url, { headers, signal: AbortSignal.timeout(30000) });
            if (!response.ok) throw new Error('HTTP ' + response.status);
            mime = response.headers.get('content-type') || '';
            if (asset.kind === 'video' && !/video|octet-stream/i.test(mime)) throw new Error('Unexpected video content type: ' + mime);
            if (asset.kind === 'image' && !/image|octet-stream/i.test(mime)) throw new Error('Unexpected image content type: ' + mime);
            const range = response.headers.get('content-range')?.match(/^bytes (\d+)-(\d+)\/(\d+)$/);
            if (response.status === 206 && (!range || Number(range[1]) !== offset)) throw new Error('Unexpected content range');
            if (response.status === 200) offset = 0;
            total = range ? Number(range[3]) : Number(response.headers.get('content-length')) || null;
            await pipeline(Readable.fromWeb(response.body), createWriteStream(target + '.part', { flags: offset ? 'a' : 'w' }));
            const stat = await fs.stat(target + '.part');
            const expected = range ? Number(range[2]) + 1 : total;
            if (!stat.size || (expected && !response.headers.get('content-encoding') && stat.size !== expected)) throw new Error('Incomplete download');
            offset = stat.size;
            if (asset.kind === 'video') console.log('MEDIA_BYTES', asset.file, offset, total);
          } while (total && offset < total);
          await fs.rename(target + '.part', target);
          asset.bytes = offset;
          asset.contentType = mime;
          lastError = null;
          break;
        } catch (e) {
          lastError = e;
          console.log('MEDIA_RETRY', asset.file, retry + 1, e.message);
          const bytesAfter = (await fs.stat(target + '.part').catch(() => null))?.size || 0;
          // An interrupted stream can still deliver a valid contiguous prefix.
          // Count consecutive stalls, rather than discarding a file making progress.
          if (asset.kind === 'video' && bytesAfter > bytesBefore) retry = -1;
        }
      }
      if (lastError) throw lastError;
      console.log('SAVED_MEDIA', asset.file, asset.bytes);
    } catch (e) {
      asset.error = e.message;
      errors.push({ file: asset.file, message: e.message });
      console.log('MEDIA_ERROR', asset.file, e.message);
    }
  }
  let nextAsset = 0;
  await Promise.all(Array.from({ length: 8 }, async () => {
    while (nextAsset < assets.length) await download(assets[nextAsset++]);
  }));
  const localize = html => {
    for (const asset of assets) if (asset.bytes) html = html.split(esc(asset.url)).join(esc(asset.file)).split(asset.url).join(asset.file);
    return html;
  };
  const sections = [];
  const markdown = ['# ' + manifest.title, '', manifest.attribution, '', 'Источник: ' + manifest.sourceUrl, '', 'Материалы получены из страниц браузера. Ответы не отправлялись. Ключи правильных ответов не извлекались.', ''];
  for (const module of manifest.modules) {
    markdown.push('## ' + module.title, '');
    let moduleHtml = '<section><h2>' + esc(module.title) + '</h2>';
    for (const lesson of module.lessons) {
      markdown.push('### ' + lesson.title, '');
      let lessonHtml = '<details open id="lesson-' + esc(lesson.id) + '"><summary>' + esc(lesson.title) + '</summary>';
      for (const step of steps.filter(s => s.lesson.id === lesson.id)) {
        const data = step.data;
        markdown.push('#### Шаг ' + data.stepNumber + ' (' + data.stepKind + ')', '', 'Источник: ' + data.sourceUrl, '', data.text, '');
        let html = '<article><h4>Шаг ' + data.stepNumber + '</h4><a href="' + esc(data.sourceUrl) + '">Оригинал в Stepik</a>' + localize(data.html);
        if (data.videos.length) {
          const video = assets.find(a => a.kind === 'video' && data.videos.includes(a.url) && a.bytes);
          if (video) {
            html += '<video controls preload="none" src="' + esc(video.file) + '"></video>';
            markdown.push('[Видео](' + video.file + ')', '');
          } else html += '<p>Видео не скачано. Смотрите оригинал.</p>';
        }
        if (data.exercise) {
          html += '<p><strong>' + esc(data.exercise.instruction) + '</strong></p>' + localize(data.exercise.contentHtml);
          html += '<p class="note">Ключ ответа не получен. Для переноса проверки нужно настроить её отдельно.</p>';
          for (const option of data.exercise.options) markdown.push('- ' + option.text);
          markdown.push('', 'Ключ ответа: отсутствует.', '');
        }
        lessonHtml += html + '</article>';
      }
      moduleHtml += lessonHtml + '</details>';
    }
    sections.push(moduleHtml + '</section>');
  }
  const toc = '<nav><ol>' + manifest.modules.flatMap(m => m.lessons.map(l => '<li><a href="#lesson-' + esc(l.id) + '">' + esc(l.title) + '</a></li>')).join('') + '</ol></nav>';
  const html = '<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; img-src \'self\' data: https:; media-src \'self\' https:; style-src \'unsafe-inline\'; base-uri \'none\'; form-action \'none\'"><title>' + esc(manifest.title) + '</title><style>body{font:17px/1.65 system-ui;margin:auto;max-width:1000px;padding:24px;color:#182132;background:#f4f6fa}article,details{background:white;padding:20px;margin:16px 0;border-radius:12px}summary{cursor:pointer;font-weight:700;font-size:22px}img,video,table{max-width:100%}video{width:100%}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:8px}.fill-blanks-quiz__input-wrapper::after{content:"[ ... ]";border-bottom:1px solid #999;padding:0 12px}.dnd-quiz__item{border:1px solid #ddd;padding:8px;margin:6px 0}.note{color:#675000;background:#fff7d9;padding:12px}a{overflow-wrap:anywhere}</style><h1>' + esc(manifest.title) + '</h1><p>' + esc(manifest.attribution) + '</p><p>Локальная копия доступных страниц курса. <a href="' + esc(manifest.sourceUrl) + '">Оригинал</a></p><p class="note">Правильные ответы и серверные проверки не получены. Сохранено шагов: ' + steps.length + '.</p>' + toc + sections.join('\n') + '</html>';
  await fs.writeFile(path.join(root, 'index.html'), html);
  await fs.writeFile(path.join(root, 'course.md'), markdown.join('\n'));
  await fs.writeFile(path.join(root, 'assets.json'), JSON.stringify(assets, null, 2) + '\n');
  const summary = { steps: steps.length, videos: assets.filter(a => a.kind === 'video' && a.bytes).length, images: assets.filter(a => a.kind === 'image' && a.bytes).length, bytes: assets.reduce((n,a) => n + (a.bytes || 0),0), errors };
  await fs.writeFile(path.join(root, 'media-summary.json'), JSON.stringify(summary, null, 2) + '\n');
  console.log('SUMMARY', JSON.stringify(summary));
  if (errors.length) process.exitCode = 1;
}
main().catch(e => { console.error(e); process.exitCode = 1; });
