#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Ставит бесплатным курсам случайную круглую цену (по умолчанию 10 000–25 000 ₽, шаг 1 000).

  SKX_MCP_URL=https://skillex.space/api/v1/mcp SKX_MCP_TOKEN=skx_... \
    python3 set_random_prices_via_mcp.py [--min 10000] [--max 25000] [--step 1000] [--dry-run]

Платные курсы не трогает. Цены по возможности не повторяются.
"""
import argparse
import random

import load_course_via_mcp as mcp


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--min', type=int, default=10000)
    ap.add_argument('--max', type=int, default=25000)
    ap.add_argument('--step', type=int, default=1000)
    ap.add_argument('--dry-run', action='store_true')
    a = ap.parse_args()

    mcp.rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'skillex-prices'}})
    found = mcp.tool('list_courses', query='')
    items = found.get('items', found) if isinstance(found, dict) else found
    free = [c for c in items if not c.get('priceCents')]
    pool = list(range(a.min, a.max + 1, a.step))
    random.shuffle(pool)
    for i, c in enumerate(free):
        price = pool[i] if i < len(pool) else random.choice(pool)
        if not a.dry_run:
            mcp.tool('update_course', course_id=c['id'], price_rub=price)
        print(f'{c["title"]}: {price:,} ₽'.replace(f'{price:,}', f'{price:,}'.replace(',', ' ')))
    print(f'курсов с новой ценой: {len(free)}' + (' (dry-run)' if a.dry_run else ''))


if __name__ == '__main__':
    main()
