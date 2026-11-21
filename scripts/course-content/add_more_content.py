#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Добавляет в курс «Цифровая гигиена» новые уроки и тесты (cifrovaya_gigiena_more.py).

Только вставка: существующие уроки не меняются (кроме сдвига позиций), прогресс
сохраняется. Транзакция защищена отпечатком текущего курса.

  python3 add_more_content.py current.json current_fp out.sql
"""
import copy
import json
import sys

sys.path.insert(0, __file__.rsplit('/', 1)[0])
import apply_cifrovaya_gigiena as A  # noqa: E402
import cifrovaya_gigiena_edits as E  # noqa: E402
import cifrovaya_gigiena_more as M  # noqa: E402


def shuffle_uniform(questions, salt):
    """Равномерная перестановка вариантов (верный ответ может оказаться на любой
    позиции, включая первую). Не трогает «Да/Нет» и тесты на сопоставление;
    «Все перечисленное»/«Ни один» остаются последними."""
    opts_sets = {tuple(q['options']) for q in questions}
    matching = len(questions) > 1 and len(opts_sets) == 1
    for q in questions:
        if matching or q['options'] in (['Да', 'Нет'], ['Нет', 'Да']):
            continue
        n = len(q['options'])
        movable = [i for i, o in enumerate(q['options']) if not A.KEEP_LAST.match(o.strip())]
        fixed = [i for i in range(n) if i not in movable]
        perm = movable[:]
        A.random.Random(f'{salt}/{q["id"]}').shuffle(perm)
        order = perm + fixed
        q['options'] = [q['options'][i] for i in order]
        q['correct'] = order.index(q['correct'])
    return questions


def build(cur):
    d = copy.deepcopy(cur)
    # Прошлое перемешивание уводило верный ответ с первой позиции — перемешиваем заново равномерно.
    for m in d['modules']:
        for l in m['lessons']:
            if l.get('quiz'):
                shuffle_uniform(l['quiz']['questions'], 'cg-2026-11-v2')
    mods = sorted(d['modules'], key=lambda m: m['position'])
    for add in M.ADDITIONS:
        mod = mods[add['module'] - 1]
        titles = [l['title'] for l in mod['lessons']]
        if add['after'] not in titles:
            raise SystemExit(f"after не найден в модуле {add['module']}: {add['after']}")
        if add['title'] in {l['title'] for m in mods for l in m['lessons']}:
            raise SystemExit(f"урок уже есть: {add['title']}")
        key = 'more/' + add['title']
        if add['kind'] == 'quiz':
            qs = [{'text': q['text'], 'options': list(q['options']), 'correct': q['correct']} for q in add['questions']]
            for q in qs:
                assert 0 <= q['correct'] < len(q['options']) and len(set(q['options'])) == len(q['options']), q
            l = A.new_lesson(mod['id'], key, add['title'], 'quiz', E.QUIZ_INTRO, quiz=qs)
            shuffle_uniform(l['quiz']['questions'], 'cg-more-2026-11')
        else:
            l = A.new_lesson(mod['id'], key, add['title'], 'video_text', A.clean_md(add['body']))
        mod['lessons'].insert(titles.index(add['after']) + 1, l)
    for m in mods:
        for i, l in enumerate(m['lessons'], start=1):
            l['position'] = i
            if l['title'].startswith('9.2 Заключение'):
                l['title'] = l['title'].replace('9.2 Заключение', '9.3 Заключение', 1)  # 9.2 — итоговый тест
    d['modules'] = mods
    return d


def to_sql(cur, new, fp):
    out = ['BEGIN;', "SET LOCAL lock_timeout = '5s';",
           'DO $$ BEGIN IF (' + A.FINGERPRINT_SQL + ') <> ' + A.lit(fp) + " THEN "
           "RAISE EXCEPTION 'Курс изменился после снимка — правки не применены'; END IF; END $$;"]
    cid = new['course']['id']
    old = {l['id']: l for m in cur['modules'] for l in m['lessons']}
    out.append(f"UPDATE lessons SET position = position + 1000 WHERE module_id IN (SELECT id FROM modules WHERE course_id={A.lit(cid)});")
    for m in new['modules']:
        for l in m['lessons']:
            if l.get('_new'):
                quiz = A.lit(json.dumps(l['quiz'], ensure_ascii=False)) + '::jsonb' if l.get('quiz') else 'NULL'
                out.append(
                    'INSERT INTO lessons (id, module_id, title, kind, position, body_md, video_url, quiz, is_preview) VALUES ('
                    f"{A.lit(l['id'])}, {A.lit(l['module_id'])}, {A.lit(l['title'])}, {A.lit(l['kind'])}, {l['position']}, "
                    f"{A.lit(l['body_md'])}, NULL, {quiz}, false);")
            else:
                sets = [f"position={l['position']}"]
                if l['title'] != old.get(l['id'], {}).get('title'):
                    sets.append(f"title={A.lit(l['title'])}")
                if l.get('quiz') != old.get(l['id'], {}).get('quiz'):
                    sets.append(f"quiz={A.lit(json.dumps(l['quiz'], ensure_ascii=False))}::jsonb")
                out.append(f"UPDATE lessons SET {', '.join(sets)} WHERE id={A.lit(l['id'])};")
    out.append('COMMIT;')
    return '\n'.join(out) + '\n'


if __name__ == '__main__':
    cur = json.load(open(sys.argv[1], encoding='utf-8'))
    new = build(cur)
    open(sys.argv[3], 'w', encoding='utf-8').write(to_sql(cur, new, sys.argv[2]))
    json.dump(new, open(sys.argv[3] + '.json', 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
    added = [l for m in new['modules'] for l in m['lessons'] if l.get('_new')]
    nq = sum(len(l['quiz']['questions']) for l in added if l.get('quiz'))
    print(f'ok: добавлено уроков {len(added)}, из них тестов {sum(1 for l in added if l["kind"] == "quiz")}, вопросов {nq}')
