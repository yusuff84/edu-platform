#!/usr/bin/env python3
"""Apply the reviewed proposal to the single imported course, preserving IDs and other data.

Requires an explicit --apply; dry-run validates the proposal and database rows.
Never prints answer keys. Keeps a verified pg_dump backup outside the repo.
"""
import argparse
import base64
import hashlib
import json
import pathlib
import subprocess
import sys
import uuid

PROPOSAL = pathlib.Path('/opt/skillex/backups/exercise-repair/repair-proposal.json')
BACKUPS = pathlib.Path('/opt/skillex/backups/exercise-repair')
SLUG = 'cifrovaya-gigiena'


def command(args, *, input=None):
    return subprocess.run(args, input=input, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)


def sql_run(sql):
    result = command(['docker', 'exec', '-i', 'skillex-db-1', 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'skillex', '-d', 'skillex', '-At'], input=sql.encode())
    return result.stdout.decode()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--apply', action='store_true', help='apply migration after a verified backup')
    args = ap.parse_args()
    doc = json.loads(PROPOSAL.read_text())
    records = doc['records']
    assert len(records) == 45 and len({r['id'] for r in records}) == 45
    assert doc['source_marker_counts'] == {'choice': 33, 'matching': 7, 'tables': 2, 'blanks': 2, 'sort': 1}
    payload = []
    for r in records:
        assert r['status'] == 'ready' and not r['pending_items']
        assert hashlib.sha256(r['source_body'].encode()).hexdigest() == r['source_body_sha256']
        questions = []
        for i, item in enumerate(r['questions']):
            choices = item['choices']
            assert len(choices) >= 2 and all(isinstance(o, str) and o.strip() for o in choices)
            assert len(set(choices)) == len(choices)
            assert isinstance(item['correct_choice'], int) and 0 <= item['correct_choice'] < len(choices)
            assert item['key_provenance'] == 'authored_review_not_original'
            questions.append({'id': str(uuid.uuid5(uuid.UUID(r['id']), 'skillex-adapted-quiz-v1:' + str(i))),
                              'text': item['prompt'], 'options': choices, 'correct': item['correct_choice']})
        assert questions
        payload.append({'id': r['id'], 'body_md5': hashlib.md5(r['source_body'].encode()).hexdigest(),
                        'quiz': {'questions': questions}})
    encoded = base64.b64encode(json.dumps(payload, ensure_ascii=False).encode()).decode()
    header = f"""BEGIN;
CREATE TEMP TABLE quiz_repair (id uuid PRIMARY KEY, body_md5 text NOT NULL, quiz jsonb NOT NULL) ON COMMIT DROP;
INSERT INTO quiz_repair SELECT (x->>'id')::uuid, x->>'body_md5', x->'quiz'
FROM jsonb_array_elements(convert_from(decode('{encoded}', 'base64'), 'UTF8')::jsonb) AS x;
DO $$ BEGIN
  IF (SELECT count(*) FROM quiz_repair) <> 45 THEN RAISE EXCEPTION 'wrong repair proposal count'; END IF;
  IF (SELECT count(*) FROM quiz_repair q JOIN lessons l ON l.id=q.id JOIN modules m ON m.id=l.module_id JOIN courses c ON c.id=m.course_id
      WHERE c.slug='{SLUG}' AND l.kind='video_text' AND l.quiz IS NULL AND l.body_md IS NOT NULL
      AND md5(l.body_md)=q.body_md5 AND jsonb_array_length(q.quiz->'questions')>0) <> 45 THEN
    RAISE EXCEPTION 'database differs from reviewed proposal; no edits applied';
  END IF;
  IF (SELECT count(*) FROM lessons l JOIN modules m ON m.id=l.module_id JOIN courses c ON c.id=m.course_id WHERE c.slug='{SLUG}') <> 83 THEN
    RAISE EXCEPTION 'course lesson count changed'; END IF;
END $$;
"""
    if not args.apply:
        print(sql_run(header + "SELECT 'dry-run validated', count(*) FROM quiz_repair; ROLLBACK;"))
        return
    BACKUPS.mkdir(mode=0o700, exist_ok=True)
    from datetime import datetime, timezone
    backup = BACKUPS / ('before-quiz-repair-' + datetime.now(timezone.utc).strftime('%Y%m%d-%H%M%S') + '.dump')
    with backup.open('wb') as out:
        dump = subprocess.run(['docker', 'exec', 'skillex-db-1', 'pg_dump', '-U', 'skillex', '-d', 'skillex', '-Fc'], stdout=out, stderr=subprocess.PIPE)
    backup.chmod(0o600)
    if dump.returncode:
        raise RuntimeError('pg_dump failed: ' + dump.stderr.decode(errors='replace'))
    with backup.open('rb') as src:
        subprocess.run(['docker', 'exec', '-i', 'skillex-db-1', 'pg_restore', '-l'], stdin=src, stdout=subprocess.DEVNULL, check=True)
    sql = header + """
UPDATE lessons l SET kind='quiz',
    body_md='**Адаптированный тест.** Вопросы и варианты восстановлены и переработаны по материалам курса. Ключи ответов составлены для этой версии, а не взяты из оригинального теста. Выберите ответы на все вопросы и отправьте на проверку.',
    quiz=q.quiz
FROM quiz_repair q WHERE l.id=q.id;
DO $$ BEGIN
  IF (SELECT count(*) FROM lessons l JOIN quiz_repair q ON l.id=q.id WHERE l.kind='quiz' AND l.quiz=q.quiz) <> 45 THEN
    RAISE EXCEPTION 'quiz update incomplete'; END IF;
END $$;
-- Prior read-only completions do not count as passing the new graded quiz.
DELETE FROM lesson_progress lp USING quiz_repair q WHERE lp.lesson_id=q.id;
SELECT 'updated-quiz-count', count(*) FROM lessons l JOIN quiz_repair q ON l.id=q.id WHERE l.kind='quiz';
COMMIT;
"""
    print('verified backup:', backup, file=sys.stderr)
    print(sql_run(sql))

if __name__ == '__main__':
    main()
