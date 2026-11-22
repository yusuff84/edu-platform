#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Загружает курс на платформу через MCP-сервер (как это делает нейросеть).

  SKX_MCP_URL=https://skillex.space/api/v1/mcp SKX_MCP_TOKEN=skx_... \
    python3 load_course_via_mcp.py sysadmin [--publish | --sync]

Источник — пакет scripts/course-content/<name>/: __init__.py c COURSE
({title, description, category, price_rub, slug}) и part*.py с MODULES
(формат — sysadmin/FORMAT.md). Модули нумеруются по порядку частей.
Если курс с таким slug уже есть — скрипт останавливается (ничего не трогает).
"""
import hashlib
import importlib
import json
import os
import random
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

URL = os.environ.get('SKX_MCP_URL', 'http://127.0.0.1:3000/api/v1/mcp')
TOKEN = os.environ['SKX_MCP_TOKEN']
_id = 0


def rpc(method, params=None):
    global _id
    _id += 1
    req = urllib.request.Request(
        URL,
        data=json.dumps({'jsonrpc': '2.0', 'id': _id, 'method': method, 'params': params or {}}).encode(),
        headers={'Content-Type': 'application/json', 'Accept': 'application/json',
                 'Authorization': 'Bearer ' + TOKEN, 'User-Agent': 'skillex-course-loader/1.0'},
        method='POST')
    with urllib.request.urlopen(req, timeout=60) as r:
        out = json.loads(r.read())
    if 'error' in out:
        raise SystemExit(f'{method}: {out["error"]}')
    return out['result']


def tool(name, **args):
    res = rpc('tools/call', {'name': name, 'arguments': args})
    text = res['content'][0]['text']
    if res.get('isError'):
        raise SystemExit(f'{name} {args.get("title", "")}: {text}')
    return json.loads(text) if text.strip().startswith(('{', '[')) else text


MEDIA_RE = re.compile(r'\]\((media/[^)\s]+)\)')
_media_cache = None


def _cache_path(pkg_dir):
    host = urllib.parse.urlparse(URL).netloc.replace(':', '_')
    return os.path.join(pkg_dir, 'media', f'.uploaded.{host}.json')


def upload_file(path, pkg_dir):
    """Загружает файл в /api/v1/mcp/media (multipart) и возвращает публичный url. Кеш — по sha256 содержимого."""
    global _media_cache
    cp = _cache_path(pkg_dir)
    if _media_cache is None:
        _media_cache = json.load(open(cp)) if os.path.exists(cp) else {}
    data = open(path, 'rb').read()
    sha = hashlib.sha256(data).hexdigest()
    if sha in _media_cache:
        return _media_cache[sha]
    boundary = '----skx' + sha[:16]
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{os.path.basename(path)}"\r\n'
            f'Content-Type: application/octet-stream\r\n\r\n').encode() + data + f'\r\n--{boundary}--\r\n'.encode()
    req = urllib.request.Request(URL.rstrip('/') + '/media', data=body, method='POST', headers={
        'Content-Type': f'multipart/form-data; boundary={boundary}', 'Authorization': 'Bearer ' + TOKEN,
        'User-Agent': 'skillex-course-loader/1.0'})
    try:
        with urllib.request.urlopen(req, timeout=300) as r:
            url = json.loads(r.read())['url']
    except urllib.error.HTTPError as e:
        raise SystemExit(f'Загрузка {path}: {e.code} {e.read()[:200]!r}')
    _media_cache[sha] = url
    os.makedirs(os.path.dirname(cp), exist_ok=True)
    json.dump(_media_cache, open(cp, 'w'), indent=1)
    print(f'  загружен {os.path.relpath(path, pkg_dir)} → {url}')
    return url


def resolve_media(body, pkg_dir):
    """![подпись](media/файл.png) → ![подпись](/uploads/media/<хеш>.png) с загрузкой файла."""
    def repl(m):
        path = os.path.join(pkg_dir, m.group(1))
        if not os.path.exists(path):
            raise SystemExit(f'Нет файла {path}')
        return '](' + upload_file(path, pkg_dir) + ')'
    return MEDIA_RE.sub(repl, body)


def module_video(m, pkg_dir):
    """Видео модуля → url. Поле 'video' — адрес уже загруженного ролика ('/uploads/media/….mp4')
    или путь к готовому mp4 внутри пакета курса (будет загружен). Ролики делаются в отдельном
    проекте skillex-video-studio — здесь видео не генерируется."""
    v = m.get('video')
    if not v:
        return ''
    if v.startswith('/uploads/') or v.startswith('https://'):
        return v
    path = os.path.join(pkg_dir, v)
    if not os.path.exists(path):
        raise SystemExit(f'Нет видеофайла {path}')
    return upload_file(path, pkg_dir)


def shuffle(questions, salt):
    """Равномерно перемешивает варианты (верный ответ — на любой позиции)."""
    out = []
    for i, q in enumerate(questions):
        opts = list(q['options'])
        order = list(range(len(opts)))
        random.Random(f'{salt}/{i}/{q["text"]}').shuffle(order)
        out.append({'text': q['text'], 'options': [opts[k] for k in order], 'correct': order.index(q['correct'])})
    return out


def sync(existing, c, modules, pkg_dir):
    """--sync: обновляет тексты уроков и видео модулей уже загруженного курса (сопоставление по названию урока).
    Структуру и тесты не трогает. Картинки media/… и видео загружаются автоматически."""
    items = existing.get('items', existing) if isinstance(existing, dict) else existing
    match = [x for x in items if x.get('slug') == c.get('slug')]
    if not match:
        raise SystemExit(f'Курс со slug {c.get("slug")} не найден — сначала загрузите его без --sync')
    cid = match[0]['id']
    src, video_of = {}, {}
    for m in modules:
        texts = [l for l in m['lessons'] if l['kind'] != 'quiz']
        for l in texts:
            src[l['title']] = resolve_media(l['body'], pkg_dir)
        if texts and m.get('video'):
            video_of[texts[0]['title']] = module_video(m, pkg_dir)
    cur = tool('get_course', course_id=cid)
    changed = []
    for m in cur['modules']:
        for l in m['lessons']:
            if l['title'] not in src:
                continue
            upd = {}
            if (l.get('bodyMd') or '').strip() != src[l['title']].strip():
                upd['body_md'] = src[l['title']]
            want_video = video_of.get(l['title'])
            if want_video and (l.get('videoUrl') or '') != want_video:
                upd['video_url'] = want_video
            if upd:
                changed.append((l, upd))
    missing = sorted(set(src) - {l['title'] for m in cur['modules'] for l in m['lessons']})
    if missing:
        print('! уроки не найдены на сайте (названия изменились?):', missing)
    if not changed:
        return print('изменений нет')
    # Контент правится только в черновике: снимаем с публикации на время правки.
    was_published = cur['course']['status'] == 'published'
    if was_published:
        tool('unpublish_course', course_id=cid)
    try:
        for l, upd in changed:
            tool('update_lesson', course_id=cid, lesson_id=l['id'], **upd)
            print(f'  обновлён: {l["title"]} ({", ".join(upd)})')
    finally:
        if was_published:
            tool('publish_course', course_id=cid)
    print(f'обновлено уроков: {len(changed)}')


def main():
    name = sys.argv[1]
    publish = '--publish' in sys.argv
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    pkg = importlib.import_module(name)
    pkg_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), name)
    parts = sorted(f[:-3] for f in os.listdir(os.path.join(os.path.dirname(os.path.abspath(__file__)), name))
                   if f.startswith('part') and f.endswith('.py'))
    modules = [m for p in parts for m in importlib.import_module(f'{name}.{p}').MODULES]
    c = pkg.COURSE

    rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'skillex-course-loader'}})
    existing = tool('list_courses', query=c['title'])
    if '--sync' in sys.argv:
        return sync(existing, c, modules, pkg_dir)
    if c.get('slug') and c['slug'] in json.dumps(existing, ensure_ascii=False):
        raise SystemExit(f'Курс со slug {c["slug"]} уже существует — загрузка остановлена (для правки текстов: --sync)')

    course = tool('create_course', title=c['title'], description=c['description'],
                  price_rub=c.get('price_rub', 0), category=c.get('category', 'other'), slug=c.get('slug', ''))
    cid = course['id']
    print(f'курс {cid} ({course.get("slug")})')
    n_lessons = n_quiz = n_q = 0
    for mi, m in enumerate(modules, start=1):
        mod = tool('add_module', course_id=cid, title=m['title'])
        for li, l in enumerate(m['lessons']):
            args = dict(course_id=cid, module_id=mod['id'], title=l['title'])
            if l['kind'] == 'quiz':
                qs = shuffle(l['questions'], f'{c["slug"]}/{mi}/{li}')
                tool('add_lesson', kind='quiz', body_md='Проверьте себя: выберите правильный ответ на каждый вопрос. '
                     'Результат появится сразу, количество попыток не ограничено.', questions=qs, **args)
                n_quiz += 1
                n_q += len(qs)
            else:
                # Первый урок курса — превью: виден без записи. Видео модуля — в его первом уроке.
                first_text = l is next(x for x in m['lessons'] if x['kind'] != 'quiz')
                video = module_video(m, pkg_dir) if first_text else ''
                tool('add_lesson', kind='video' if video else 'text', body_md=resolve_media(l['body'], pkg_dir),
                     is_preview=(mi == 1 and li == 0), **({'video_url': video} if video else {}), **args)
            n_lessons += 1
        print(f'  модуль {mi}: {m["title"]} — {len(m["lessons"])} шагов')
    if publish:
        tool('publish_course', course_id=cid)
        print('опубликован')
    print(f'итого: модулей {len(modules)}, шагов {n_lessons}, тестов {n_quiz}, вопросов {n_q}')


if __name__ == '__main__':
    main()
