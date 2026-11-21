#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Редакция курса «Цифровая гигиена» (ноябрь 2026): строит SQL-миграцию контента.

Вход: JSON-снимок курса (см. --dump-sql ниже, как его получить).
Выход: SQL, который в ОДНОЙ транзакции
  1) проверяет отпечаток текущего контента курса — если курс успели изменить
     после снимка, транзакция откатывается и ничего не меняется;
  2) обновляет описание курса, названия модулей, тексты/тесты уроков;
  3) добавляет новые уроки (id детерминированы — повторный запуск не плодит дубли,
     но отпечаток после первого применения уже другой, так что повтор просто откатится).

Прогресс учеников сохраняется: существующие уроки обновляются по id, не пересоздаются.

Использование:
  python3 apply_cifrovaya_gigiena.py snapshot.json fingerprint out.sql [report.md]
Снимок и отпечаток:
  psql -At -c "$(python3 apply_cifrovaya_gigiena.py --snapshot-sql)" > snapshot.json
  psql -At -c "$(python3 apply_cifrovaya_gigiena.py --fingerprint-sql)"
"""
import copy
import json
import random
import re
import sys
import uuid

sys.path.insert(0, __file__.rsplit('/', 1)[0])
import cifrovaya_gigiena_edits as E  # noqa: E402
import cifrovaya_gigiena_new as N  # noqa: E402

SLUG = 'cifrovaya-gigiena'
NS = uuid.UUID('6f1c1d1e-2b1e-4d39-9a52-cf0c2d5a2026')

SNAPSHOT_SQL = (
    "select json_build_object('course',(select row_to_json(c) from courses c where c.slug='" + SLUG + "'),"
    "'modules',(select json_agg(json_build_object('id',m.id,'title',m.title,'position',m.position,"
    "'lessons',(select json_agg(row_to_json(l) order by l.position) from lessons l where l.module_id=m.id))"
    " order by m.position) from modules m join courses c on c.id=m.course_id where c.slug='" + SLUG + "'))"
)
FINGERPRINT_SQL = (
    "select md5(coalesce((select string_agg(l.id::text||'|'||l.title||'|'||l.position||'|'||coalesce(l.body_md,'')"
    "||'|'||coalesce(l.quiz::text,''), '#' order by l.id) from lessons l join modules m on m.id=l.module_id"
    " join courses c on c.id=m.course_id where c.slug='" + SLUG + "'),'')"
    "||(select string_agg(m.id::text||m.title||m.position, '#' order by m.id) from modules m join courses c"
    " on c.id=m.course_id where c.slug='" + SLUG + "'))"
)

TYPOS = [
    ('ВКонтаке ', 'ВКонтакте '), ('ВКонтаке,', 'ВКонтакте,'), ('ВКонтаке.', 'ВКонтакте.'),
    ('взаимодействие в государственными', 'взаимодействие с государственными'),
    ('с лицам нацеленным', 'с лицами, нацеленными'),
    ('Остаетесь ли вы безопасности?', 'Остаётесь ли вы в безопасности?'),
    ('в данном случает', 'в данном случае'),
    ('Москва сити', '«Москва-Сити»'),
    ('на USB-накопителя', 'на USB-накопитель'),
    ('в последствии', 'впоследствии'),
    ('В заключении', 'В заключение'),
    ('невозможно .', 'невозможно.'), ('пользователя .', 'пользователя.'),
    ('Сбербанка', 'Сбербанка'), ('сбербанка', 'Сбербанка'),
    ('​', ''),
]


def lid(key):
    return str(uuid.uuid5(NS, key))


def clean_md(body):
    lines = []
    for ln in body.replace('\r\n', '\n').split('\n'):
        if re.fullmatch(r'\s*#{1,6}\s*', ln):
            continue  # пустые заголовки «## »
        ln = re.sub(r'^(#{1,6})\s*\*\*(.+?)\*\*\s*$', r'\1 \2', ln)
        lines.append(ln)
    s = '\n'.join(lines)
    # **жирный**слово → **жирный** слово (в исходнике пропущены пробелы)
    s = re.sub(r'(\*\*[^*\n]+?\*\*)(?=[0-9A-Za-zА-Яа-яЁё])', r'\1 ', s)
    for a, b in TYPOS:
        s = s.replace(a, b)
    s = re.sub(r'\bIOS\b', 'iOS', s)
    s = re.sub(r'\n{3,}', '\n\n', s)
    return s.strip() + '\n'


KEEP_LAST = re.compile(r'^(все\b|ни од|каждый образец|всё перечисл)', re.I)


def shuffle_quiz(questions, salt):
    """Перемешивает варианты, чтобы верный ответ не стоял почти всегда первым.
    Не трогает «Да/Нет», тесты на сопоставление (общий список вариантов у всех
    вопросов) и оставляет «Все перечисленное»/«Ни один» последними."""
    opts_sets = {tuple(q['options']) for q in questions}
    matching = len(questions) > 1 and len(opts_sets) == 1
    for q in questions:
        if matching or q['options'] in (['Да', 'Нет'], ['Нет', 'Да']):
            continue
        n = len(q['options'])
        movable = [i for i, o in enumerate(q['options']) if not KEEP_LAST.match(o.strip())]
        fixed = [i for i in range(n) if i not in movable]
        rnd = random.Random(f'{salt}/{q["id"]}')
        perm = movable[:]
        for _ in range(10):
            rnd.shuffle(perm)
            if len(perm) < 2 or perm.index(q['correct']) != 0 or q['correct'] not in movable:
                break
        order = perm + fixed
        q['options'] = [q['options'][i] for i in order]
        q['correct'] = order.index(q['correct'])
    return questions


def rename(title, kind, seen):
    m = re.match(r'^(.*?) — шаг (\d+)\. (Видео|Текст|Задание)$', title)
    if not m:
        return title
    base, step, typ = m.group(1), int(m.group(2)), m.group(3)
    if typ == 'Видео':
        label = 'видео'
    elif typ == 'Текст':
        label = 'конспект'
    else:
        seen.setdefault(base, 0)
        seen[base] += 1
        return f'{base} · задание {seen[base]}'
    key = (base, label)
    seen.setdefault(key, 0)
    seen[key] += 1
    return f'{base} · {label}' + (f' (часть {seen[key]})' if seen[key] > 1 else '')


def build(snapshot):
    d = copy.deepcopy(snapshot)
    course = d['course']
    mods = d['modules']
    by_title = {l['title']: l for m in mods for l in m['lessons']}
    changed_titles = []

    def L(t):
        if t not in by_title:
            raise SystemExit(f'Урок не найден: {t}')
        return by_title[t]

    # --- Точечные правки и переписанные уроки (до переименования) ---
    for title, old, new in E.REPLACE:
        l = L(title)
        if old not in l['body_md']:
            raise SystemExit(f'Фрагмент не найден в «{title}»: {old[:60]}')
        l['body_md'] = l['body_md'].replace(old, new, 1)
    for title, ids in E.REMOVE_IMAGES.items():
        l = L(title)
        before = l['body_md']
        l['body_md'] = '\n'.join(x for x in before.split('\n') if not any(i in x for i in ids))
        assert l['body_md'] != before, title
    t, start, end, block = E.SOCIAL_BLOCK
    l = L(t)
    b = l['body_md']
    i, j = b.index(start), b.index(end)
    l['body_md'] = b[:i] + block.strip() + '\n\n' + b[j:]
    a, bnew = E.SOCIAL_INTRO
    assert a in l['body_md']
    l['body_md'] = l['body_md'].replace(a, bnew)
    l['body_md'] = l['body_md'].replace('мы разберем в будущих уроках', 'мы разберём в модуле про социальную инженерию')
    # 5.2: неподтверждённое «iOS — идеальная ОС для слежки»
    l52 = L('5.2 Вредоносное ПО для смартфонов — шаг 2. Текст')
    l52['body_md'] = re.sub(r'[^.!?\n]*идеальн[^.!?\n]*[.!?]\s*', '', l52['body_md'])
    L('2.1 Цифровой профиль и из чего он состоит — шаг 2. Текст')['body_md'] = E.DIGITAL_PROFILE
    L('7.1 Законодательство в сфере информационной безопасности — шаг 2. Текст')['body_md'] = E.LAW

    for t, fn in E.QUIZ_FIX.items():
        l = L(t)
        l['quiz']['questions'] = fn(l['quiz']['questions'])

    # --- Модуль 8: практический модуль вместо обрывков ---
    m8 = next(m for m in mods if m['title'].lstrip('0123456789. ').startswith('Как защищаться'))
    les = m8['lessons']
    v1, texts, v2 = les[0], les[1:7], les[7]
    new8 = [
        ('8.1 Чек-лист цифровой гигиены', N.CHECKLIST),
        ('8.2 Обновления и резервные копии', N.UPDATES_BACKUPS),
        ('8.3 Смартфон и публичный Wi‑Fi', N.SMARTPHONE),
        ('8.4 VPN и мессенджеры: что важно знать', N.VPN_MESSENGERS),
        ('8.5 Если вас взломали или украли деньги', N.INCIDENT_PLAN),
        ('8.6 Самозапрет на кредиты и другие инструменты защиты', N.SELF_BAN),
    ]
    for l, (title, body) in zip(texts, new8):
        l['title'], l['body_md'], l['video_url'], l['quiz'] = title, strip_h1(body), None, None
        l['_fixed_title'] = True
    family = new_lesson(m8['id'], 'm8/family', '8.7 Как защитить детей и пожилых родственников', 'video_text', N.FAMILY)
    quiz8 = new_lesson(m8['id'], 'm8/quiz', '8.8 Практикум: как защищаться', 'quiz', E.QUIZ_INTRO,
                       quiz=N.QUIZ_MODULE8)
    v1['title'], v2['title'] = '8.0 Цифровая гигиена · видео', '8.0 Цифровая гигиена · видео (часть 2)'
    v1['_fixed_title'] = v2['_fixed_title'] = True
    m8['lessons'] = [v1] + texts + [family, v2, quiz8]

    # --- Модуль 3: современное мошенничество ---
    m3 = next(m for m in mods if 'Последствия взлома' in m['title'])
    k = max(i for i, l in enumerate(m3['lessons']) if l['title'].startswith('3.2 '))
    fraud = new_lesson(m3['id'], 'm3/fraud2026', '3.3 Мошенничество сегодня: как распознать за 10 секунд',
                       'video_text', N.FRAUD_2026)
    m3['lessons'].insert(k + 1, fraud)
    for l in m3['lessons']:
        l['title'] = l['title'].replace('3.3 Практикум', '3.4 Практикум')

    # --- Модуль 4: 2FA, менеджеры паролей, passkeys ---
    m4 = next(m for m in mods if 'без взлома' in m['title'])
    k = max(i for i, l in enumerate(m4['lessons']) if l['title'].startswith('4.2 '))
    acc = new_lesson(m4['id'], 'm4/2fa', '4.3 Двухфакторная аутентификация, менеджеры паролей и ключи доступа',
                     'video_text', N.ACCOUNTS_2FA)
    m4['lessons'].insert(k + 1, acc)
    for l in m4['lessons']:
        l['title'] = l['title'].replace('4.4 Практикум', '4.5 Практикум').replace('4.3 Кибербуллинг', '4.4 Кибербуллинг')

    # --- Итоговый тест: шаг по новым темам перед заключительным видео ---
    m9 = mods[-1]
    k = max(i for i, l in enumerate(m9['lessons']) if l['kind'] == 'quiz')
    extra = new_lesson(m9['id'], 'm9/extra', '9.1 Итоговое тестирование — шаг 17. Задание', 'quiz', E.QUIZ_INTRO,
                       quiz=N.QUIZ_FINAL_EXTRA)
    m9['lessons'].insert(k + 1, extra)

    # --- Общие правки всех уроков ---
    for m in mods:
        m['title'] = re.sub(r'^\d+[.\s]+', '', m['title']).strip()
        seen = {}
        for pos, l in enumerate(m['lessons'], start=1):
            l['position'] = pos
            if l['kind'] == 'quiz':
                l['body_md'] = E.QUIZ_INTRO
                for qi, q in enumerate(l['quiz']['questions']):
                    q.setdefault('id', lid(f'{l["id"]}/q{qi}'))
                    if not q.get('id'):
                        q['id'] = lid(f'{l["id"]}/q{qi}')
                shuffle_quiz(l['quiz']['questions'], 'cg-2026-11')
            elif l.get('body_md'):
                l['body_md'] = clean_md(l['body_md'])
            if not l.pop('_fixed_title', False):
                l['title'] = rename(l['title'], l['kind'], seen)
    course['description'] = E.COURSE_DESCRIPTION
    return d


def strip_h1(body):
    """Первая строка «# Заголовок» дублирует название урока над плеером — убираем."""
    lines = body.strip().split('\n')
    if lines and lines[0].startswith('# '):
        lines = lines[1:]
    return '\n'.join(lines).strip() + '\n'


def new_lesson(module_id, key, title, kind, body, quiz=None):
    if kind != 'quiz':
        body = strip_h1(body)
    l = {'id': lid(key), 'module_id': module_id, 'title': title, 'kind': kind, 'position': 0,
         'body_md': body, 'video_url': None, 'quiz': None, 'is_preview': False, '_new': True}
    if quiz is not None:
        qs = copy.deepcopy(quiz)
        for i, q in enumerate(qs):
            q['id'] = lid(f'{key}/q{i}')
        l['quiz'] = {'questions': qs}
    return l


def lit(v):
    if v is None:
        return 'NULL'
    return "'" + str(v).replace("'", "''") + "'"


def to_sql(old, new, fingerprint):
    out = ['BEGIN;', "SET LOCAL lock_timeout = '5s';",
           'DO $$ BEGIN IF (' + FINGERPRINT_SQL + ") <> " + lit(fingerprint) + " THEN "
           "RAISE EXCEPTION 'Курс изменился после снимка — правки не применены'; END IF; END $$;"]
    cid = new['course']['id']
    out.append(f"UPDATE courses SET description={lit(new['course']['description'])}, updated_at=now() WHERE id={lit(cid)};")
    # Два прохода позиций из-за UNIQUE (module_id, position).
    out.append(f"UPDATE lessons SET position = position + 1000 WHERE module_id IN (SELECT id FROM modules WHERE course_id={lit(cid)});")
    old_l = {l['id']: l for m in old['modules'] for l in m['lessons']}
    for m in new['modules']:
        out.append(f"UPDATE modules SET title={lit(m['title'])} WHERE id={lit(m['id'])};")
        for l in m['lessons']:
            quiz = lit(json.dumps(l['quiz'], ensure_ascii=False)) + '::jsonb' if l.get('quiz') else 'NULL'
            if l.get('_new'):
                out.append(
                    'INSERT INTO lessons (id, module_id, title, kind, position, body_md, video_url, quiz, is_preview) VALUES ('
                    f"{lit(l['id'])}, {lit(l['module_id'])}, {lit(l['title'])}, {lit(l['kind'])}, {l['position']}, "
                    f"{lit(l['body_md'])}, NULL, {quiz}, false);")
            else:
                o = old_l[l['id']]
                sets = [f"position={l['position']}"]
                if l['title'] != o['title']:
                    sets.append(f"title={lit(l['title'])}")
                if (l.get('body_md') or '') != (o.get('body_md') or ''):
                    sets.append(f"body_md={lit(l['body_md'])}")
                if l.get('quiz') != o.get('quiz'):
                    sets.append(f"quiz={quiz}")
                if l.get('video_url') != o.get('video_url'):
                    sets.append(f"video_url={lit(l.get('video_url'))}")
                out.append(f"UPDATE lessons SET {', '.join(sets)} WHERE id={lit(l['id'])};")
    out.append('COMMIT;')
    return '\n'.join(out) + '\n'


def report(old, new):
    old_l = {l['id']: l for m in old['modules'] for l in m['lessons']}
    rows = []
    for m in new['modules']:
        rows.append(f"\n## {m['title']}")
        for l in m['lessons']:
            o = old_l.get(l['id'])
            if l.get('_new'):
                tag = 'НОВЫЙ'
            else:
                ch = [n for n, a, b in (('название', o['title'], l['title']), ('текст', o.get('body_md'), l.get('body_md')),
                                        ('тест', o.get('quiz'), l.get('quiz'))) if a != b]
                tag = ', '.join(ch) if ch else '—'
            rows.append(f"- {l['title']}  [{tag}]")
    return '\n'.join(rows)


if __name__ == '__main__':
    if sys.argv[1] == '--snapshot-sql':
        print(SNAPSHOT_SQL)
        sys.exit()
    if sys.argv[1] == '--fingerprint-sql':
        print(FINGERPRINT_SQL)
        sys.exit()
    snap = json.load(open(sys.argv[1], encoding='utf-8'))
    fp = sys.argv[2]
    new = build(snap)
    open(sys.argv[3], 'w', encoding='utf-8').write(to_sql(snap, new, fp))
    if len(sys.argv) > 4:
        open(sys.argv[4], 'w', encoding='utf-8').write(report(snap, new))
    json.dump(new, open(sys.argv[3] + '.json', 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
    print('ok')
