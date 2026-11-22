#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Сборка видеоурока: слайды (HTML → PNG) + озвучка (Piper TTS) → MP4 до 5 МБ.

  python3 video/build_video.py sysadmin/videos/m01.py [--voice dmitri] [--keep]

Файл сценария — Python-модуль с VIDEO = {
    'title': 'Модуль 1. ...', 'footer': 'Модуль 1', 'kicker': 'Модуль 1',
    'slides': [ {'layout': 'title'|'points'|'terminal'|'compare'|'steps'|'term'|'diagram'|'tip'|'summary',
                 ...поля макета..., 'say': 'текст озвучки (кириллицей, числа словами)'} ],
}
Результат — рядом со сценарием: <имя>.mp4 и <имя>.vtt (субтитры по слайдам).
Нужны Docker (образ skx-tts из video/Dockerfile.tts — соберётся сам) и frontend/node_modules (Playwright).
"""
import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '..', '..', '..'))
FRONTEND = os.path.join(ROOT, 'frontend')
VOICES_DIR = os.path.join(HERE, '.voices')  # не в git (см. .gitignore)
IMAGE = 'skx-tts'
MAX_BYTES = 5_400_000
THEMES = {
    'sysadmin': {'bg1': '#151a4a', 'bg2': '#2a2a9e', 'bg3': '#4635ff', 'accent': '#fec466'},
    'gigiena': {'bg1': '#07403f', 'bg2': '#0d7a6f', 'bg3': '#19b394', 'accent': '#fec466'},
}

COVERS = {
    'sysadmin': os.path.join(HERE, '..', 'covers', 'sysadmin-basics.jpg'),
    'gigiena': os.path.join(HERE, '..', 'covers', 'cifrovaya-gigiena.jpg'),
}

# Слова, которые чаще всего пишут латиницей, — на всякий случай заменяем (основное правило — писать кириллицей).
SPEAK = {
    'Windows': 'виндоус', 'Linux': 'линукс', 'Ubuntu': 'убунту', 'DHCP': 'ди-эйч-си-пи', 'DNS': 'ди-эн-эс',
    'IP': 'ай-пи', 'USB': 'ю-эс-би', 'Wi-Fi': 'вай-фай', 'SSD': 'эс-эс-ди', 'HDD': 'эйч-ди-ди', 'BIOS': 'биос',
    'UEFI': 'юи-эф-ай', 'RAM': 'оперативная память', 'CPU': 'процессор', 'PowerShell': 'пауэршелл', 'SSH': 'эс-эс-эйч',
    'VPN': 'ви-пи-эн', 'MAC': 'мак', 'LAN': 'лан', 'VLAN': 'ви-лан', 'NAT': 'нат', 'TCP': 'ти-си-пи', 'UDP': 'ю-ди-пи',
}


def load_spec(path):
    spec = importlib.util.spec_from_file_location('video_spec', path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod.VIDEO


def lint(video):
    errs = []
    for i, sl in enumerate(video['slides'], 1):
        say = sl.get('say', '').strip()
        if not say:
            errs.append(f'слайд {i}: нет текста озвучки (say)')
            continue
        fixed = say
        for k, v in SPEAK.items():
            fixed = re.sub(rf'(?<![A-Za-z]){re.escape(k)}(?![A-Za-z])', v, fixed)
        sl['_say'] = fixed
        if re.search(r'[A-Za-z]', fixed):
            errs.append(f'слайд {i}: латиница в озвучке: {re.findall(r"[A-Za-z][A-Za-z.-]*", fixed)[:5]}')
        if re.search(r'\d', fixed):
            errs.append(f'слайд {i}: цифры в озвучке (пишите словами): {re.findall(r"[0-9][0-9.,/:-]*", fixed)[:5]}')
    return errs


def ensure_image():
    if subprocess.run(['docker', 'image', 'inspect', IMAGE], capture_output=True).returncode != 0:
        subprocess.run(['docker', 'build', '-q', '-t', IMAGE, '-f', os.path.join(HERE, 'Dockerfile.tts'), HERE], check=True)


def ensure_voice(name):
    os.makedirs(VOICES_DIR, exist_ok=True)
    base = f'ru_RU-{name}-medium'
    for ext in ('onnx', 'onnx.json'):
        p = os.path.join(VOICES_DIR, f'{base}.{ext}')
        if not os.path.exists(p):
            url = f'https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/ru/ru_RU/{name}/medium/{base}.{ext}'
            print('скачиваю голос', url)
            urllib.request.urlretrieve(url, p)
    return base


def docker(work, cmd):
    return subprocess.run(['docker', 'run', '--rm', '-v', f'{work}:/w', '-v', f'{VOICES_DIR}:/voices', IMAGE, 'sh', '-c', cmd],
                          check=True, capture_output=True, text=True).stdout


def vtt_time(t):
    h, rem = divmod(t, 3600)
    m, s = divmod(rem, 60)
    return f'{int(h):02d}:{int(m):02d}:{s:06.3f}'


def main():
    spec_path = os.path.abspath(sys.argv[1])
    voice = sys.argv[sys.argv.index('--voice') + 1] if '--voice' in sys.argv else 'dmitri'
    theme = sys.argv[sys.argv.index('--theme') + 1] if '--theme' in sys.argv else 'sysadmin'
    video = load_spec(spec_path)
    errs = lint(video)
    if errs:
        raise SystemExit('Сценарий не прошёл проверку:\n  ' + '\n  '.join(errs))
    ensure_image()
    base = ensure_voice(voice)
    work = tempfile.mkdtemp(prefix='skxvid-')
    try:
        # 1. Слайды.
        cover = video.get('cover') or COVERS.get(theme)
        meta = {'course': video.get('course', ''), 'footer': video.get('footer', ''), 'kicker': video.get('kicker', ''),
                'theme': THEMES.get(theme, {}), 'cover': ('file://' + cover) if cover and os.path.exists(cover) else ''}
        with open(os.path.join(work, 'spec.json'), 'w') as f:
            json.dump({'meta': meta, 'slides': video['slides']}, f, ensure_ascii=False)
        r = subprocess.run(['node', 'scripts/render-slides.mjs', os.path.join(work, 'spec.json'), work], cwd=FRONTEND,
                           capture_output=True, text=True)
        print(r.stdout.strip())
        if r.returncode != 0 or '!' in r.stdout:
            raise SystemExit('Ошибка рендера слайдов:\n' + r.stdout + r.stderr)
        # 2. Озвучка: по файлу на слайд.
        n = len(video['slides'])
        for i, sl in enumerate(video['slides'], 1):
            with open(os.path.join(work, f'say_{i:02d}.txt'), 'w') as f:
                f.write(sl['_say'].strip() + '\n')
        ids = ' '.join(f'{i:02d}' for i in range(1, n + 1))
        out = docker(work, f'for i in {ids}; do piper -m /voices/{base}.onnx --sentence_silence 0.35 --length_scale 1.04 '
                           f'-f /w/say_$i.wav < /w/say_$i.txt 2>/dev/null || exit 1; '
                           f'ffprobe -v error -show_entries format=duration -of csv=p=0 /w/say_$i.wav; done')
        durs = [float(x) for x in out.split()]
        if len(durs) != n:
            raise SystemExit(f'Озвучка: ожидалось {n} фрагментов, получено {len(durs)}')
        # 3. Сегменты: картинка + речь, пауза 0.5 c до и 0.7 c после, мягкое появление/исчезание.
        segs = []
        for i, d in enumerate(durs, 1):
            total = d + 1.2
            seg = f'seg_{i:02d}.mp4'
            fade_out = max(0.0, total - 0.35)
            docker(work, f'ffmpeg -y -v error -loop 1 -framerate 12 -i /w/slide_{i:02d}.png -i /w/say_{i:02d}.wav '
                         f'-filter_complex "[0:v]scale=1280:720:flags=lanczos,format=yuv420p,fade=t=in:st=0:d=0.3,fade=t=out:st={fade_out:.2f}:d=0.35[v];'
                         f'[1:a]adelay=500|500,apad,atrim=0:{total:.2f},aresample=24000[a]" -map "[v]" -map "[a]" '
                         f'-t {total:.2f} -c:v libx264 -preset slow -tune stillimage -crf 30 -r 12 -g 120 '
                         f'-c:a aac -b:a 56k -ac 1 /w/{seg}')
            segs.append((seg, total))
        with open(os.path.join(work, 'list.txt'), 'w') as f:
            f.writelines(f"file '{s}'\n" for s, _ in segs)
        docker(work, 'ffmpeg -y -v error -f concat -safe 0 -i /w/list.txt -c copy -movflags +faststart /w/out.mp4')
        out_mp4 = os.path.splitext(spec_path)[0] + '.mp4'
        shutil.copy(os.path.join(work, 'out.mp4'), out_mp4)
        # Постер — первый слайд.
        shutil.copy(os.path.join(work, 'slide_01.png'), os.path.splitext(spec_path)[0] + '.poster.png')
        # 4. Субтитры по слайдам.
        t = 0.0
        lines = ['WEBVTT', '']
        for i, ((_, total), sl) in enumerate(zip(segs, video['slides']), 1):
            lines += [str(i), f'{vtt_time(t + 0.5)} --> {vtt_time(t + total - 0.5)}', sl['say'].strip(), '']
            t += total
        with open(os.path.splitext(spec_path)[0] + '.vtt', 'w') as f:
            f.write('\n'.join(lines))
        size = os.path.getsize(out_mp4)
        print(f'готово: {out_mp4} — {t / 60:.1f} мин, {size / 1e6:.2f} МБ')
        if size > MAX_BYTES:
            raise SystemExit(f'Файл больше {MAX_BYTES / 1e6:.1f} МБ — сократите озвучку или число слайдов')
    finally:
        if '--keep' not in sys.argv:
            shutil.rmtree(work, ignore_errors=True)
        else:
            print('временные файлы:', work)


if __name__ == '__main__':
    main()
