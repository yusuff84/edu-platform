#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Итоговая редакция курса «Цифровая гигиена»: одна актуальная версия каждой темы.

Решение автора: две версии одного материала в курсе оставлять нельзя — остаётся
наиболее актуальная. Скрипт приводит курс к редакции apply_cifrovaya_gigiena.py
(исправленные авторские уроки + новые материалы на месте устаревших) и удаляет
временные дубли модуля 8 (m8b/*), созданные restore_author_content.py.

  python3 finalize_current_revision.py original.json current.json current_fp out.sql
    original.json — снимок курса ДО редакции (вход apply_cifrovaya_gigiena.py)
"""
import json
import sys

sys.path.insert(0, __file__.rsplit('/', 1)[0])
import apply_cifrovaya_gigiena as A  # noqa: E402


def to_sql(cur, target, fp):
    out = ['BEGIN;', "SET LOCAL lock_timeout = '5s';",
           'DO $$ BEGIN IF (' + A.FINGERPRINT_SQL + ') <> ' + A.lit(fp) + " THEN "
           "RAISE EXCEPTION 'Курс изменился после снимка — правки не применены'; END IF; END $$;"]
    cid = target['course']['id']
    c_by_id = {l['id']: l for m in cur['modules'] for l in m['lessons']}
    t_ids = {l['id'] for m in target['modules'] for l in m['lessons']}
    for lid_ in sorted(set(c_by_id) - t_ids):
        out.append(f"DELETE FROM lessons WHERE id={A.lit(lid_)};")
    out.append(f"UPDATE courses SET description={A.lit(target['course']['description'])}, updated_at=now() WHERE id={A.lit(cid)};")
    out.append(f"UPDATE lessons SET position = position + 1000 WHERE module_id IN (SELECT id FROM modules WHERE course_id={A.lit(cid)});")
    for m in target['modules']:
        out.append(f"UPDATE modules SET title={A.lit(m['title'])} WHERE id={A.lit(m['id'])};")
        for l in m['lessons']:
            quiz = A.lit(json.dumps(l['quiz'], ensure_ascii=False)) + '::jsonb' if l.get('quiz') else 'NULL'
            if l['id'] not in c_by_id:
                out.append(
                    'INSERT INTO lessons (id, module_id, title, kind, position, body_md, video_url, quiz, is_preview) VALUES ('
                    f"{A.lit(l['id'])}, {A.lit(l['module_id'])}, {A.lit(l['title'])}, {A.lit(l['kind'])}, {l['position']}, "
                    f"{A.lit(l['body_md'])}, {A.lit(l.get('video_url'))}, {quiz}, {str(bool(l.get('is_preview'))).lower()});")
                continue
            out.append(
                f"UPDATE lessons SET module_id={A.lit(l['module_id'])}, position={l['position']}, title={A.lit(l['title'])}, "
                f"body_md={A.lit(l.get('body_md'))}, video_url={A.lit(l.get('video_url'))}, quiz={quiz} WHERE id={A.lit(l['id'])};")
    out.append('COMMIT;')
    return '\n'.join(out) + '\n'


if __name__ == '__main__':
    orig = json.load(open(sys.argv[1], encoding='utf-8'))
    cur = json.load(open(sys.argv[2], encoding='utf-8'))
    target = A.build(orig)
    open(sys.argv[4], 'w', encoding='utf-8').write(to_sql(cur, target, sys.argv[3]))
    json.dump(target, open(sys.argv[4] + '.json', 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
    print('ok')
