#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Применяет правки текстов уроков {lessonId: bodyMd} к курсу через MCP.

  SKX_MCP_URL=… SKX_MCP_TOKEN=… python3 apply_edits_via_mcp.py <slug> <edits.json> [--check course.json]

Опубликованный курс на время правки снимается с публикации и сразу публикуется обратно.
С --check сначала прогоняется check_same_words.py (правки «только оформление»).
"""
import json
import subprocess
import sys

import load_course_via_mcp as mcp


def main():
    slug, path = sys.argv[1], sys.argv[2]
    edits = json.load(open(path))
    if '--check' in sys.argv:
        orig = sys.argv[sys.argv.index('--check') + 1]
        if subprocess.run([sys.executable, 'check_same_words.py', orig, path]).returncode != 0:
            raise SystemExit('Проверка «только оформление» не пройдена — ничего не применено')
    mcp.rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'skillex-edits'}})
    found = mcp.tool('list_courses', query='')
    items = found.get('items', found) if isinstance(found, dict) else found
    course = next((c for c in items if c.get('slug') == slug), None)
    if not course:
        raise SystemExit(f'Курс {slug} не найден')
    cur = mcp.tool('get_course', course_id=course['id'])
    lessons = {l['id']: l for m in cur['modules'] for l in m['lessons']}
    todo = [(lid, body) for lid, body in edits.items() if lid in lessons and (lessons[lid].get('bodyMd') or '').strip() != body.strip()]
    unknown = [lid for lid in edits if lid not in lessons]
    if unknown:
        raise SystemExit(f'Неизвестные уроки: {unknown[:5]}')
    if not todo:
        return print('изменений нет')
    was_published = cur['course']['status'] == 'published'
    if was_published:
        mcp.tool('unpublish_course', course_id=course['id'])
    try:
        for lid, body in todo:
            mcp.tool('update_lesson', course_id=course['id'], lesson_id=lid, body_md=body)
            print('  обновлён:', lessons[lid]['title'])
    finally:
        if was_published:
            mcp.tool('publish_course', course_id=course['id'])
    print(f'обновлено уроков: {len(todo)}')


if __name__ == '__main__':
    main()
