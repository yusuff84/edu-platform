#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Проверка «только оформление»: новые тексты уроков содержат ровно те же слова
в том же порядке, что и исходные. Разрешено добавлять только разметку
(выноски, **жирный**, ==маркер==, заголовки ##, списки, разделители) и короткие
заголовки выносок (строка «> [!ТИП] Заголовок» — до 6 слов).

  python3 check_same_words.py <course.json от get_course> <edits.json {lessonId: newBody}>
"""
import json
import re
import sys

WORD = re.compile(r'[0-9A-Za-zА-Яа-яЁё]+')
CALLOUT_HEAD = re.compile(r'^\s*>\s*\[![A-Za-zА-Яа-яЁё]+\]\s*(.*)$')


def words(md, allow_titles=False):
    out = []
    titles = []
    for line in md.replace('\r\n', '\n').split('\n'):
        m = CALLOUT_HEAD.match(line)
        if m:
            titles.append(m.group(1))
            continue
        out.extend(w.lower().replace('ё', 'е') for w in WORD.findall(line))
    return out, titles


def main():
    course = json.load(open(sys.argv[1]))
    edits = json.load(open(sys.argv[2]))
    orig = {l['id']: (l['title'], l.get('bodyMd') or '') for m in course['modules'] for l in m['lessons']}
    bad = 0
    for lid, body in edits.items():
        if lid not in orig:
            print(f'✗ {lid}: нет такого урока')
            bad += 1
            continue
        title, src = orig[lid]
        a, _ = words(src)
        b, titles = words(body)
        long_titles = [t for t in titles if len(WORD.findall(t)) > 6]
        if a != b or long_titles:
            bad += 1
            i = next((k for k in range(min(len(a), len(b))) if a[k] != b[k]), min(len(a), len(b)))
            print(f'✗ {title}: слова отличаются с позиции {i}: было «{" ".join(a[max(0, i - 4):i + 6])}», стало «{" ".join(b[max(0, i - 4):i + 6])}»'
                  + (f'; длинные заголовки выносок: {long_titles}' if long_titles else ''))
        else:
            print(f'✓ {title}')
    print(f'\nпроверено {len(edits)}, ошибок {bad}')
    sys.exit(1 if bad else 0)


if __name__ == '__main__':
    main()
