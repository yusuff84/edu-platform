#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Ставит обложку курсу через MCP (инструмент set_course_cover).

  SKX_MCP_URL=https://skillex.space/api/v1/mcp SKX_MCP_TOKEN=skx_... \
    python3 set_cover_via_mcp.py <slug> <картинка.jpg|png|webp>

Обложки лежат в covers/: <slug>.jpg — готовая картинка 1600×1000,
<slug>.html — её исходник (SVG; перерисовать — открыть в браузере и снять скриншот 1600×1000).
"""
import base64
import json
import sys

import load_course_via_mcp as mcp


def main():
    slug, path = sys.argv[1], sys.argv[2]
    mcp.rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'skillex-cover'}})
    found = mcp.tool('list_courses', query='')
    items = found.get('items', found) if isinstance(found, dict) else found
    course = next((c for c in items if c.get('slug') == slug), None)
    if not course:
        raise SystemExit(f'Курс со slug {slug} не найден')
    with open(path, 'rb') as f:
        data = base64.b64encode(f.read()).decode()
    res = mcp.tool('set_course_cover', course_id=course['id'], image_base64=data)
    print(course['title'], '→', json.dumps(res, ensure_ascii=False)[:200])


if __name__ == '__main__':
    main()
