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
import importlib
import json
import os
import random
import sys
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


def shuffle(questions, salt):
    """Равномерно перемешивает варианты (верный ответ — на любой позиции)."""
    out = []
    for i, q in enumerate(questions):
        opts = list(q['options'])
        order = list(range(len(opts)))
        random.Random(f'{salt}/{i}/{q["text"]}').shuffle(order)
        out.append({'text': q['text'], 'options': [opts[k] for k in order], 'correct': order.index(q['correct'])})
    return out


def sync(existing, c, modules):
    """--sync: обновляет тексты уроков уже загруженного курса (сопоставление по названию урока).
    Структуру и тесты не трогает — только body текстовых уроков, которые изменились."""
    items = existing.get('items', existing) if isinstance(existing, dict) else existing
    match = [x for x in items if x.get('slug') == c.get('slug')]
    if not match:
        raise SystemExit(f'Курс со slug {c.get("slug")} не найден — сначала загрузите его без --sync')
    cid = match[0]['id']
    src = {l['title']: l['body'] for m in modules for l in m['lessons'] if l['kind'] != 'quiz'}
    cur = tool('get_course', course_id=cid)
    changed = [(l, src[l['title']]) for m in cur['modules'] for l in m['lessons']
               if l['title'] in src and (l.get('bodyMd') or '').strip() != src[l['title']].strip()]
    if not changed:
        return print('изменений нет')
    # Контент правится только в черновике: снимаем с публикации на время правки.
    was_published = cur['course']['status'] == 'published'
    if was_published:
        tool('unpublish_course', course_id=cid)
    for l, body in changed:
        tool('update_lesson', course_id=cid, lesson_id=l['id'], body_md=body)
        print(f'  обновлён: {l["title"]}')
    if was_published:
        tool('publish_course', course_id=cid)
    print(f'обновлено уроков: {len(changed)}')


def main():
    name = sys.argv[1]
    publish = '--publish' in sys.argv
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    pkg = importlib.import_module(name)
    parts = sorted(f[:-3] for f in os.listdir(os.path.join(os.path.dirname(os.path.abspath(__file__)), name))
                   if f.startswith('part') and f.endswith('.py'))
    modules = [m for p in parts for m in importlib.import_module(f'{name}.{p}').MODULES]
    c = pkg.COURSE

    rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'skillex-course-loader'}})
    existing = tool('list_courses', query=c['title'])
    if '--sync' in sys.argv:
        return sync(existing, c, modules)
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
                # Первый урок курса — превью: виден без записи.
                tool('add_lesson', kind='text', body_md=l['body'], is_preview=(mi == 1 and li == 0), **args)
            n_lessons += 1
        print(f'  модуль {mi}: {m["title"]} — {len(m["lessons"])} шагов')
    if publish:
        tool('publish_course', course_id=cid)
        print('опубликован')
    print(f'итого: модулей {len(modules)}, шагов {n_lessons}, тестов {n_quiz}, вопросов {n_q}')


if __name__ == '__main__':
    main()
