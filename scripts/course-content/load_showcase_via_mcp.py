#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Загружает витринные курсы (showcase/__init__.py) через MCP: курс, модули, уроки-заглушки, публикация, обложка.

  SKX_MCP_URL=https://skillex.space/api/v1/mcp SKX_MCP_TOKEN=skx_... python3 load_showcase_via_mcp.py [slug…]

Курсы, чей slug уже есть на платформе, пропускаются (ничего не трогаем).
"""
import base64
import json
import os
import sys

import load_course_via_mcp as mcp
from showcase import COURSES, PLACEHOLDER

COVERS = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'covers')


def main():
    only = set(sys.argv[1:])
    mcp.rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'skillex-showcase'}})
    found = mcp.tool('list_courses', query='')
    items = found.get('items', found) if isinstance(found, dict) else found
    existing = {c.get('slug') for c in items}
    for c in COURSES:
        if only and c['slug'] not in only:
            continue
        if c['slug'] in existing:
            print(f'пропуск {c["slug"]}: уже есть')
            continue
        course = mcp.tool('create_course', title=c['title'], description=c['description'], price_rub=0,
                          category=c['category'], slug=c['slug'])
        cid = course['id']
        n = 0
        for mi, (mtitle, lessons) in enumerate(c['modules']):
            mod = mcp.tool('add_module', course_id=cid, title=mtitle)
            for li, ltitle in enumerate(lessons):
                mcp.tool('add_lesson', course_id=cid, module_id=mod['id'], title=ltitle, kind='text',
                         body_md=PLACEHOLDER, is_preview=(mi == 0 and li == 0))
                n += 1
        mcp.tool('publish_course', course_id=cid)
        with open(os.path.join(COVERS, c['slug'] + '.jpg'), 'rb') as f:
            mcp.tool('set_course_cover', course_id=cid, image_base64=base64.b64encode(f.read()).decode())
        print(f'{c["slug"]}: модулей {len(c["modules"])}, уроков {n} — опубликован с обложкой')


if __name__ == '__main__':
    main()
