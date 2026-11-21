"""Import the local browser export into the development platform (no Stepik API).

Copies media to the API's persistent uploads volume, then inserts one course in
a PostgreSQL transaction. Stable IDs and DO NOTHING preserve subsequent edits.
Exercises are reading material, never quizzes with fabricated answer keys.
"""
import argparse
import json
import re
import subprocess
import uuid
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import quote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
MEDIA_PREFIX = '/uploads/imports/stepik-146168/'
SLUG = 'cifrovaya-gigiena'


class Node:
    def __init__(self, tag='', attrs=()):
        self.tag, self.attrs, self.children = tag, dict(attrs), []


class Document(HTMLParser):
    VOID = {'img', 'input', 'br', 'hr', 'meta', 'link', 'source', 'wbr', 'area', 'embed', 'col'}

    def __init__(self, source):
        super().__init__(convert_charrefs=True)
        self.root = Node()
        self.stack = [self.root]
        self.feed(source)

    def handle_starttag(self, tag, attrs):
        node = Node(tag, attrs)
        self.stack[-1].children.append(node)
        if tag not in self.VOID:
            self.stack.append(node)

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        if tag not in self.VOID:
            self.handle_endtag(tag)

    def handle_endtag(self, tag):
        for i in range(len(self.stack) - 1, 0, -1):
            if self.stack[i].tag == tag:
                self.stack = self.stack[:i]
                break

    def handle_data(self, text):
        self.stack[-1].children.append(text)


def identifier(value):
    return str(uuid.uuid5(uuid.NAMESPACE_URL, 'https://stepik.org/course/146168/' + value))


def run(command, **kwargs):
    result = subprocess.run(command, capture_output=True, encoding='utf-8', **kwargs)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    return result.stdout.strip()


def convert(node, media):
    if isinstance(node, str):
        return re.sub(r'\s+', ' ', node)
    if node.tag in {'script', 'style', 'svg', 'button', 'input', 'textarea'}:
        return ''
    if 'fill-blanks-quiz__input-wrapper' in (node.attrs.get('class') or ''):
        return ' [____] '
    body = ''.join(convert(child, media) for child in node.children)
    if node.tag == 'img':
        url = node.attrs.get('src', '')
        if url not in media:
            raise ValueError('Image has no local media file: ' + url)
        alt = (node.attrs.get('alt') or 'Иллюстрация').replace('[', '').replace(']', '')
        return '\n\n![' + alt + '](' + media[url] + ')\n\n'
    if node.tag == 'a':
        url = node.attrs.get('href', '')
        if urlsplit(url).scheme not in {'http', 'https'}:
            return body
        url = quote(url, safe=':/?#[]@!$&\'+,;=%~._-')
        return '[' + (body.strip() or url).replace('[', '').replace(']', '') + '](' + url + ')'
    if re.fullmatch(r'h[1-6]', node.tag):
        return '\n\n' + '#' * int(node.tag[1]) + ' ' + body.strip() + '\n\n'
    if node.tag in {'b', 'strong'}:
        return '**' + body.strip() + '**' if body.strip() else ''
    if node.tag in {'em', 'i'}:
        return '*' + body.strip() + '*' if body.strip() else ''
    if node.tag == 'br':
        return '\n'
    if node.tag in {'ul', 'ol'}:
        items = [c for c in node.children if isinstance(c, Node) and c.tag == 'li']
        return '\n\n' + '\n'.join((str(i + 1) + '. ' if node.tag == 'ol' else '- ') + convert(c, media).strip() for i, c in enumerate(items)) + '\n\n'
    if node.tag == 'tr':
        cells = [convert(c, media).strip() for c in node.children if isinstance(c, Node) and c.tag in {'td', 'th'}]
        return '\n- ' + ' | '.join(c for c in cells if c) + '\n'
    if node.tag in {'p', 'div', 'section', 'table', 'blockquote'}:
        return '\n\n' + body.strip() + '\n\n' if body.strip() else ''
    return body


def markdown(source, media, first_only=False):
    document = Document(source)
    node = document.root
    if first_only:
        node = next((n for n in node.children if isinstance(n, Node)), node)
    return re.sub(r'\n[ \t]*\n(?:[ \t]*\n)+', '\n\n', convert(node, media)).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--export-dir', type=Path, default=ROOT / '.local/stepik-146168')
    parser.add_argument('--db-container', default='edu-review-db-1')
    parser.add_argument('--api-container', default='edu-review-api-1')
    parser.add_argument('--db-user', default='edu')
    parser.add_argument('--db-name', default='edu')
    parser.add_argument('--teacher-id', type=uuid.UUID)
    parser.add_argument('--prepare-only', action='store_true')
    args = parser.parse_args()
    root = args.export_dir.resolve()
    manifest = json.loads((root / 'manifest.json').read_text(encoding='utf-8'))
    assets = json.loads((root / 'assets.json').read_text(encoding='utf-8'))
    media = {}
    for asset in assets:
        local = (root / asset['file']).resolve()
        if not local.is_relative_to(root / 'assets') or not local.is_file():
            raise ValueError('Missing or unsafe asset path')
        if local.stat().st_size != asset.get('bytes') or asset.get('error'):
            raise ValueError('Incomplete media: ' + asset['file'])
        media[asset['url']] = MEDIA_PREFIX + local.name
    course_id = identifier('course')
    modules, lessons = [], []
    exercise_count = 0
    for mi, module in enumerate(manifest['modules'], 1):
        module_id = identifier('module/' + module['id'])
        modules.append(dict(id=module_id, title=module['title'], position=mi))
        position = 0
        for lesson in module['lessons']:
            refs = lesson['steps']
            if [s['number'] for s in refs] != list(range(1, lesson['expectedStepCount'] + 1)):
                raise ValueError('Incomplete lesson ' + lesson['id'])
            for ref in refs:
                position += 1
                step_path = (root / ref['file']).resolve()
                if not step_path.is_relative_to(root / 'steps'):
                    raise ValueError('Unsafe step path')
                step = json.loads(step_path.read_text(encoding='utf-8'))
                exercise = step.get('exercise')
                body = markdown(step['html'], media, first_only=bool(exercise))
                if exercise:
                    exercise_count += 1
                    body += '\n\n**' + exercise['instruction'] + '**\n\n' + markdown(exercise['contentHtml'], media)
                    body = '**Самостоятельное задание.** Автоматическая проверка пока не настроена: авторский ключ ответа отсутствует. Отметка о прохождении означает ознакомление с заданием.\n\n' + body
                body += '\n\n[Источник: Stepik](' + step['sourceUrl'] + ')'
                video = next((media[u] for u in step['videos'] if u in media), None)
                if step['videos'] and not video:
                    raise ValueError('Video missing from local assets')
                label = 'Задание' if exercise else 'Видео' if video else 'Текст'
                lessons.append(dict(id=identifier('lesson/' + lesson['id'] + '/step/' + str(ref['number'])), module_id=module_id,
                                    title=lesson['title'] + ' — шаг ' + str(ref['number']) + '. ' + label,
                                    position=position, body_md=body, video_url=video, is_preview=mi == 1 and not exercise))
    if len(modules) != 9 or len(lessons) != 83 or exercise_count != 45:
        raise ValueError('Unexpected source structure')
    if sum(a['kind'] == 'video' for a in assets) != 18:
        raise ValueError('Expected all 18 local videos')
    cover = next((media[a['url']] for a in assets if a['kind'] == 'image'), None)
    payload = dict(course_id=course_id, slug=SLUG, title=manifest['title'], cover=cover,
                   description='Бесплатный курс по цифровой гигиене и кибербезопасности: защита аккаунтов, пароли, социальные сети, вредоносное ПО, OSINT и безопасное поведение в интернете. 9 разделов, 23 исходных урока, 83 шага. Видео 720p и материалы доступны на нашей платформе.\n\n45 заданий доступны для самостоятельной работы; автоматическая проверка не настроена, авторские ключи ответов не получены.\n\n' + manifest['attribution'] + '\nОригинал: ' + manifest['sourceUrl'] + '\nЛицензия исходных открытых материалов: CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/). Адаптация: материалы разделены на шаги, задания перенесены для самостоятельной работы.',
                   modules=modules, lessons=lessons)
    (root / 'platform-import.json').write_text(json.dumps(payload, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    if args.prepare_only:
        print(json.dumps(dict(course_id=course_id, modules=len(modules), lessons=len(lessons), exercises=exercise_count)))
        return
    psql = ['docker', 'exec', '-i', args.db_container, 'psql', '-U', args.db_user, '-d', args.db_name, '-v', 'ON_ERROR_STOP=1', '-At']
    owner = str(args.teacher_id) if args.teacher_id else run(psql + ['-c', "SELECT id FROM users WHERE role IN ('admin','teacher') AND status='active' ORDER BY (role='admin') DESC,created_at LIMIT 1;"])
    owner = str(uuid.UUID(owner))
    run(['docker', 'exec', '-u', '0', args.api_container, 'mkdir', '-p', MEDIA_PREFIX])
    run(['docker', 'cp', str(root / 'assets') + '/.', args.api_container + ':' + MEDIA_PREFIX])
    run(['docker', 'exec', '-u', '0', args.api_container, 'chown', '-R', '1000:1000', MEDIA_PREFIX])
    tag = '$import_' + uuid.uuid4().hex + '$'
    encoded = json.dumps(payload, ensure_ascii=False)
    assert tag not in encoded
    sql = f"""
BEGIN;
SELECT pg_advisory_xact_lock(146168, 1);
CREATE TEMP TABLE stepik_payload(data jsonb) ON COMMIT DROP;
INSERT INTO stepik_payload VALUES ({tag}{encoded}{tag}::jsonb);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM users WHERE id='{owner}' AND role IN ('teacher','admin') AND status='active') THEN
    RAISE EXCEPTION 'Owner must be an active teacher/admin';
  END IF;
  IF EXISTS (SELECT 1 FROM courses WHERE slug='{SLUG}' AND id <> '{course_id}') THEN
    RAISE EXCEPTION 'Slug belongs to another course';
  END IF;
END $$;
INSERT INTO courses(id,teacher_id,slug,title,description,cover_path,price_cents,currency,status,category)
SELECT (data->>'course_id')::uuid,'{owner}'::uuid,data->>'slug',data->>'title',data->>'description',data->>'cover',0,'RUB','published','other'
FROM stepik_payload ON CONFLICT(id) DO NOTHING;
INSERT INTO modules(id,course_id,title,position)
SELECT x.id,'{course_id}'::uuid,x.title,x.position FROM stepik_payload,
jsonb_to_recordset(data->'modules') AS x(id uuid,title text,position int) ON CONFLICT(id) DO NOTHING;
INSERT INTO lessons(id,module_id,title,kind,position,body_md,video_url,is_preview)
SELECT x.id,x.module_id,x.title,'video_text',x.position,x.body_md,x.video_url,x.is_preview FROM stepik_payload,
jsonb_to_recordset(data->'lessons') AS x(id uuid,module_id uuid,title text,position int,body_md text,video_url text,is_preview boolean)
ON CONFLICT(id) DO NOTHING;
COMMIT;
SELECT json_build_object('courseId',c.id,'slug',c.slug,'status',c.status,'modules',count(DISTINCT m.id),'steps',count(l.id),'videos',count(l.video_url))
FROM courses c JOIN modules m ON m.course_id=c.id JOIN lessons l ON l.module_id=m.id WHERE c.id='{course_id}' GROUP BY c.id;
"""
    result = run(psql, input=sql)
    print(result)
    report = dict(courseId=course_id, slug=SLUG, dbContainer=args.db_container, apiContainer=args.api_container, modules=len(modules), steps=len(lessons), videos=18, exercises=exercise_count)
    (root / 'platform-import-result.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
