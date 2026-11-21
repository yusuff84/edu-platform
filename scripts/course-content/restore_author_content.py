#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Возврат авторского контента курса «Цифровая гигиена» после редакции 2026-11.

Решение автора курса: его тексты не переписываем. Этот скрипт
- возвращает исходные тексты уроков, которые редакция меняла по смыслу
  (1.1, 2.1, 2.2, 3.1, 3.2, 4.1, 5.1, 5.2, 6.1, 6.2, 7.1 и шесть текстов модуля 8),
  оставляя только правки вёрстки и опечаток (clean_md);
- возвращает исходный тест 2.3 (шаг 1), который был переделан под новый текст 2.1;
- новые материалы модуля 8 переносит в ОТДЕЛЬНЫЕ уроки после авторских —
  ничего авторского они больше не заменяют.
Остальные дополнения (уроки 3.3 и 4.3, практикум модуля 8, шаг итогового теста,
перемешанные варианты и исправленные тесты) сохраняются.

  python3 restore_author_content.py original.json current.json current_fp out.sql
"""
import json
import sys

sys.path.insert(0, __file__.rsplit('/', 1)[0])
import apply_cifrovaya_gigiena as A  # noqa: E402
import cifrovaya_gigiena_edits as E  # noqa: E402

REWRITTEN_TITLES = {t for t, _, _ in E.REPLACE} | set(E.REMOVE_IMAGES) | {
    E.SOCIAL_BLOCK[0],
    '2.1 Цифровой профиль и из чего он состоит — шаг 2. Текст',
    '7.1 Законодательство в сфере информационной безопасности — шаг 2. Текст',
    '5.2 Вредоносное ПО для смартфонов — шаг 2. Текст',
}
QUIZ_RESTORE_TITLES = {'2.3 Практикум — шаг 1. Задание'}

# Материалы редакции для модуля 8 — отдельными уроками после авторских.
M8_NEW = [
    ('m8b/checklist', '8.2 Чек-лист цифровой гигиены', A.N.CHECKLIST),
    ('m8b/updates', '8.3 Обновления и резервные копии', A.N.UPDATES_BACKUPS),
    ('m8b/smartphone', '8.4 Смартфон и публичный Wi‑Fi', A.N.SMARTPHONE),
    ('m8b/vpn', '8.5 VPN и мессенджеры: что важно знать', A.N.VPN_MESSENGERS),
    ('m8b/incident', '8.6 Если вас взломали или украли деньги', A.N.INCIDENT_PLAN),
    ('m8b/selfban', '8.7 Самозапрет на кредиты и другие инструменты защиты', A.N.SELF_BAN),
]


def build(orig, cur):
    o_by_id = {l['id']: l for m in orig['modules'] for l in m['lessons']}
    o_by_title = {l['title']: l for l in o_by_id.values()}
    new = json.loads(json.dumps(cur))
    c_by_id = {l['id']: l for m in new['modules'] for l in m['lessons']}

    for t in REWRITTEN_TITLES:
        o = o_by_title[t]
        c_by_id[o['id']]['body_md'] = A.clean_md(o['body_md'])
    for t in QUIZ_RESTORE_TITLES:
        o = o_by_title[t]
        c_by_id[o['id']]['quiz'] = o['quiz']

    # Модуль 8: авторские уроки — на свои места, с исходными текстами.
    om8 = next(m for m in orig['modules'] if 'Как защищаться' in m['title'])
    m8 = next(m for m in new['modules'] if m['id'] == om8['id'])
    seen = {}
    author = []
    for ol in om8['lessons']:
        cl = c_by_id[ol['id']]
        cl['title'] = A.rename(ol['title'], ol['kind'], seen)
        cl['body_md'] = A.clean_md(ol['body_md']) if ol.get('body_md') else ol.get('body_md')
        cl['video_url'], cl['quiz'] = ol.get('video_url'), ol.get('quiz')
        author.append(cl)
    author_ids = {l['id'] for l in author}
    extra = [A.new_lesson(m8['id'], key, title, 'video_text', body) for key, title, body in M8_NEW]
    rest = {l['title'][:4]: l for l in m8['lessons'] if l['id'] not in author_ids}
    family = next(l for l in m8['lessons'] if l['id'] == A.lid('m8/family'))
    quiz8 = next(l for l in m8['lessons'] if l['id'] == A.lid('m8/quiz'))
    family['title'] = '8.8 Как защитить детей и пожилых родственников'
    quiz8['title'] = '8.9 Практикум: как защищаться'
    m8['lessons'] = author + extra + [family, quiz8]
    for i, l in enumerate(m8['lessons'], start=1):
        l['position'] = i
    del rest
    return new


def to_sql(cur, new, fp):
    out = ['BEGIN;', "SET LOCAL lock_timeout = '5s';",
           'DO $$ BEGIN IF (' + A.FINGERPRINT_SQL + ') <> ' + A.lit(fp) + " THEN "
           "RAISE EXCEPTION 'Курс изменился после снимка — правки не применены'; END IF; END $$;"]
    cid = new['course']['id']
    out.append(f"UPDATE lessons SET position = position + 1000 WHERE module_id IN (SELECT id FROM modules WHERE course_id={A.lit(cid)});")
    c_by_id = {l['id']: l for m in cur['modules'] for l in m['lessons']}
    for m in new['modules']:
        for l in m['lessons']:
            quiz = A.lit(json.dumps(l['quiz'], ensure_ascii=False)) + '::jsonb' if l.get('quiz') else 'NULL'
            if l.get('_new') and l['id'] not in c_by_id:
                out.append(
                    'INSERT INTO lessons (id, module_id, title, kind, position, body_md, video_url, quiz, is_preview) VALUES ('
                    f"{A.lit(l['id'])}, {A.lit(l['module_id'])}, {A.lit(l['title'])}, {A.lit(l['kind'])}, {l['position']}, "
                    f"{A.lit(l['body_md'])}, NULL, {quiz}, false);")
                continue
            o = c_by_id[l['id']]
            sets = [f"position={l['position']}"]
            if l['title'] != o['title']:
                sets.append(f"title={A.lit(l['title'])}")
            if (l.get('body_md') or '') != (o.get('body_md') or ''):
                sets.append(f"body_md={A.lit(l['body_md'])}")
            if l.get('quiz') != o.get('quiz'):
                sets.append(f"quiz={quiz}")
            if l.get('video_url') != o.get('video_url'):
                sets.append(f"video_url={A.lit(l.get('video_url'))}")
            out.append(f"UPDATE lessons SET {', '.join(sets)} WHERE id={A.lit(l['id'])};")
    out.append('COMMIT;')
    return '\n'.join(out) + '\n'


if __name__ == '__main__':
    orig = json.load(open(sys.argv[1], encoding='utf-8'))
    cur = json.load(open(sys.argv[2], encoding='utf-8'))
    new = build(orig, cur)
    open(sys.argv[4], 'w', encoding='utf-8').write(to_sql(cur, new, sys.argv[3]))
    json.dump(new, open(sys.argv[4] + '.json', 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
    print('ok')
