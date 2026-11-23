#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Генерирует обложки витринных курсов: ../covers/<slug>.html (SVG 1600×1000, светлый стиль на белом фоне).

  python3 make_covers.py && (cd ../../../frontend && node ../scripts/course-content/showcase/render_covers.mjs)

Композиция держится в «безопасной зоне» x 220–1380, y 90–640: карточка каталога обрезает края,
а низ закрывают затемнение и название курса.
"""
import os
import random

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'covers')
MONO = "Menlo, 'SF Mono', 'DejaVu Sans Mono', monospace"
SANS = "-apple-system, 'Segoe UI', Inter, Roboto, Arial, sans-serif"
INK = '#1b2140'
LINE = '#e4e7f2'


def g(x, y, body, shadow=True, extra=''):
    f = ' filter="url(#shadow)"' if shadow else ''
    return f'<g transform="translate({x} {y}){extra}"{f}>{body}</g>'


def t(x, y, s, fill=INK, size=26, font=SANS, weight=700, anchor='start'):
    return (f'<text x="{x}" y="{y}" fill="{fill}" font-size="{size}" font-family="{font}" '
            f'font-weight="{weight}" text-anchor="{anchor}">{s}</text>')


def bar(x, y, w, h=12, fill=INK, op=1.0):
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{h / 2}" fill="{fill}" fill-opacity="{op}"/>'


def card(w, h, inner='', rx=24, fill='#ffffff'):
    return (f'<rect width="{w}" height="{h}" rx="{rx}" fill="{fill}"/>'
            f'<rect x="0.75" y="0.75" width="{w - 1.5}" height="{h - 1.5}" rx="{rx}" fill="none" stroke="{LINE}" stroke-width="1.5"/>'
            + inner)


def chip(label, dot=None, w=None, size=28, font=SANS, fill='#ffffff', color=INK):
    w = w or (len(label) * size * 0.62 + (86 if dot else 56))
    inner = ''
    x = w / 2
    if dot:
        inner += f'<circle cx="40" cy="40" r="13" fill="{dot}"/>'
        x = 40 + 26 + (w - 66 - 24) / 2
    inner += t(x, 40 + size * 0.36, label, color, size, font, 800, 'middle')
    if fill == '#ffffff':
        return card(w, 80, inner, 40)
    return f'<rect width="{w}" height="80" rx="40" fill="{fill}"/>' + inner


def darkwin(w, h, body, fill='#141a33'):
    return (f'<rect width="{w}" height="{h}" rx="24" fill="{fill}"/>'
            '<circle cx="32" cy="32" r="8" fill="#ff5f57"/><circle cx="58" cy="32" r="8" fill="#febc2e"/>'
            '<circle cx="84" cy="32" r="8" fill="#28c840"/>' + body)


def lightwin(w, h, body):
    return card(w, h, f'<path d="M0 24 a24 24 0 0 1 24 -24 h{w - 48} a24 24 0 0 1 24 24 v30 h-{w} z" fill="#f3f4fa"/>'
                '<circle cx="32" cy="27" r="8" fill="#ff5f57"/><circle cx="58" cy="27" r="8" fill="#febc2e"/>'
                '<circle cx="84" cy="27" r="8" fill="#28c840"/>' + body)


def gear(col, hole='#ffffff'):
    return (f'<g fill="{col}" transform="scale(3)"><path d="M0 -18 l4 0 l1.5 5 a13 13 0 0 1 4 2.3 l5 -1.6 l2 3.4 l-3.8 3.6 '
            'a13 13 0 0 1 0 4.6 l3.8 3.6 l-2 3.4 l-5 -1.6 a13 13 0 0 1 -4 2.3 l-1.5 5 l-8 0 l-1.5 -5 a13 13 0 0 1 -4 -2.3 '
            'l-5 1.6 l-2 -3.4 l3.8 -3.6 a13 13 0 0 1 0 -4.6 l-3.8 -3.6 l2 -3.4 l5 1.6 a13 13 0 0 1 4 -2.3 l1.5 -5 z"/>'
            f'<circle r="5.5" fill="{hole}"/></g>')


def heart(s, col):
    return (f'<path transform="scale({s})" d="M0 30 C -40 0, -40 -30, -20 -38 C -8 -43, 0 -34, 0 -26 '
            f'C 0 -34, 8 -43, 20 -38 C 40 -30, 40 0, 0 30 Z" fill="{col}"/>')


def star(s, col):
    return (f'<path transform="scale({s})" d="M0 -40 C 4 -10, 10 -4, 40 0 C 10 4, 4 10, 0 40 '
            f'C -4 10, -10 4, -40 0 C -10 -4, -4 -10, 0 -40 Z" fill="{col}"/>')


def dashed(paths, color):
    return (f'<g stroke="{color}" stroke-opacity="0.45" stroke-width="3" fill="none" stroke-linecap="round" '
            'stroke-dasharray="2 12">' + ''.join(f'<path d="{d}"/>' for d in paths) + '</g>')


def page(a1, a2, art, extra_defs=''):
    """a1, a2 — акцентные цвета курса: ими тонированы мягкие пятна на белом фоне."""
    return f'''<!doctype html>
<html><head><meta charset="utf-8"><style>
html,body{{margin:0;padding:0;background:#fff}}
svg{{display:block}}
</style></head><body>
<svg width="1600" height="1000" viewBox="0 0 1600 1000" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <radialGradient id="blob1" cx="0.5" cy="0.5" r="0.5">
      <stop offset="0" stop-color="{a1}" stop-opacity="0.16"/><stop offset="1" stop-color="{a1}" stop-opacity="0"/>
    </radialGradient>
    <radialGradient id="blob2" cx="0.5" cy="0.5" r="0.5">
      <stop offset="0" stop-color="{a2}" stop-opacity="0.14"/><stop offset="1" stop-color="{a2}" stop-opacity="0"/>
    </radialGradient>
    <pattern id="dots" width="34" height="34" patternUnits="userSpaceOnUse">
      <circle cx="2" cy="2" r="1.7" fill="#1b2140" fill-opacity="0.07"/>
    </pattern>
    <filter id="shadow" x="-60%" y="-60%" width="220%" height="240%">
      <feDropShadow dx="0" dy="14" stdDeviation="20" flood-color="#1b2559" flood-opacity="0.13"/>
    </filter>
    {extra_defs}
  </defs>
  <rect width="1600" height="1000" fill="#ffffff"/>
  <rect width="1600" height="1000" fill="url(#dots)"/>
  <ellipse cx="560" cy="300" rx="560" ry="420" fill="url(#blob1)"/>
  <ellipse cx="1150" cy="560" rx="560" ry="440" fill="url(#blob2)"/>
  <g transform="translate(0 50)">{art}</g>
</svg>
</body></html>
'''


def grad(gid, c1, c2, vertical=False):
    x2, y2 = ('0', '1') if vertical else ('1', '1')
    return (f'<linearGradient id="{gid}" x1="0" y1="0" x2="{x2}" y2="{y2}">'
            f'<stop offset="0" stop-color="{c1}"/><stop offset="1" stop-color="{c2}"/></linearGradient>')


# ---------- 1. Python ----------
def python():
    B, Y = '#3776ab', '#ffcf3f'
    code = (t(44, 110, '<tspan fill="#c792ea">def</tspan> <tspan fill="#82aaff">greet</tspan>(name):', '#e9ecff', 30, MONO, 500)
            + t(44, 162, '    <tspan fill="#c792ea">return</tspan> <tspan fill="#c3e88d">f"Привет, {name}!"</tspan>', '#e9ecff', 30, MONO, 500)
            + t(44, 238, '<tspan fill="#82aaff">print</tspan>(greet(<tspan fill="#c3e88d">"мир"</tspan>))', '#e9ecff', 30, MONO, 500)
            + t(44, 314, f'<tspan fill="{Y}">&gt;&gt;&gt;</tspan> Привет, мир!', '#9fb4d8', 30, MONO, 500)
            + '<rect x="44" y="346" width="16" height="30" fill="#e9ecff"/>')
    art = dashed(['M470 520 C 400 540, 380 560, 360 590', 'M1150 260 C 1200 250, 1220 230, 1240 220'], B)
    art += g(480, 150, darkwin(680, 410, code))
    art += g(420, 200, f'<circle r="96" fill="{Y}"/>' + t(0, 20, '.py', INK, 60, SANS, 900, 'middle'))
    art += g(1090, 240, chip('[1, 2, 3]', font=MONO, color=B, size=32))
    art += g(1130, 470, chip('{ k: v }', font=MONO, color=B, size=32))
    art += g(250, 560, chip('for i in range', font=MONO, fill=B, color='#ffffff', size=28))
    return page(B, Y, art)


# ---------- 2. Web ----------
def web():
    O, T = '#ff7a45', '#18a999'
    body = (bar(120, 18, 380, 18, INK, .08) + t(140, 33, 'skillex-site.ru', '#7b829e', 16, MONO, 500)
            + bar(40, 86, 100, 14, O) + bar(470, 88, 60, 10, INK, .25) + bar(545, 88, 60, 10, INK, .25) + bar(620, 88, 50, 10, INK, .25)
            + f'<rect x="40" y="128" width="640" height="170" rx="18" fill="{T}"/>'
            + bar(76, 172, 300, 24, '#ffffff', .95) + bar(76, 212, 220, 14, '#ffffff', .7)
            + f'<rect x="76" y="244" width="140" height="32" rx="16" fill="{O}"/>'
            + '<circle cx="590" cy="214" r="54" fill="#ffffff" fill-opacity="0.22"/>'
            + ''.join(f'<rect x="{40 + i * 220}" y="318" width="200" height="120" rx="16" fill="#f3f4fa"/>'
                      f'<rect x="{60 + i * 220}" y="338" width="54" height="42" rx="10" fill="{c}"/>'
                      + bar(60 + i * 220, 396, 140, 10, INK, .3) + bar(60 + i * 220, 414, 90, 10, INK, .15)
                      for i, c in enumerate(['#4cc9f0', O, '#4ade80'])))
    art = dashed(['M490 470 C 420 490, 390 520, 370 550', 'M1210 200 C 1240 200, 1250 220, 1260 240'], T)
    art += g(480, 140, lightwin(720, 460, body))
    art += g(430, 190, f'<circle r="92" fill="{O}"/>' + t(0, 20, '&lt;/&gt;', '#ffffff', 54, MONO, 800, 'middle'))
    for i, (label, col) in enumerate([('HTML', '#e34c26'), ('CSS', '#2965f1'), ('JS', '#e0b000')]):
        art += g(1150, 230 + i * 110, chip(label, col, 190))
    art += g(250, 560, chip('display: flex', font=MONO, fill=INK, color='#7ff0e0', size=26))
    return page(O, T, art)


# ---------- 3. VR / AR ----------
def vr():
    V, P, C = '#7c3aed', '#ec4899', '#22d3ee'
    defs = (grad('hs', '#8b5cf6', '#6d28d9', True)
            + '<radialGradient id="lens" cx="0.35" cy="0.35" r="0.8"><stop offset="0" stop-color="#a5f3fc"/>'
              '<stop offset="0.5" stop-color="#38bdf8"/><stop offset="1" stop-color="#4338ca"/></radialGradient>')
    grid = f'<g stroke="{V}" stroke-opacity="0.13" stroke-width="2">'
    for i in range(-9, 10):
        grid += f'<line x1="{800 + i * 34}" y1="560" x2="{800 + i * 150}" y2="1000"/>'
    for y in (580, 615, 665, 735, 830, 960):
        grid += f'<line x1="0" y1="{y}" x2="1600" y2="{y}"/>'
    grid += '</g>'
    headset = ('<path d="M0 90 C -90 90, -120 140, -120 190" stroke="#4c1d95" stroke-width="36" fill="none" stroke-linecap="round"/>'
               '<path d="M600 90 C 690 90, 720 140, 720 190" stroke="#4c1d95" stroke-width="36" fill="none" stroke-linecap="round"/>'
               '<rect x="0" y="0" width="600" height="300" rx="96" fill="url(#hs)"/>'
               '<rect x="30" y="30" width="540" height="240" rx="74" fill="#1e1140"/>'
               '<ellipse cx="170" cy="150" rx="104" ry="86" fill="url(#lens)"/>'
               '<ellipse cx="430" cy="150" rx="104" ry="86" fill="url(#lens)"/>'
               '<ellipse cx="140" cy="120" rx="28" ry="17" fill="#ffffff" fill-opacity="0.6"/>'
               '<ellipse cx="400" cy="120" rx="28" ry="17" fill="#ffffff" fill-opacity="0.6"/>'
               '<path d="M262 300 q38 -46 76 0" fill="#ffffff"/>')

    def cube(s, col):
        a = s * 0.5
        return (f'<path d="M0 {a} L{s} 0 L{2 * s} {a} L{s} {s} Z" fill="{col}" fill-opacity="0.18"/>'
                f'<g fill="none" stroke="{col}" stroke-width="5" stroke-linejoin="round">'
                f'<path d="M0 {a} L{s} 0 L{2 * s} {a} L{s} {s} Z"/>'
                f'<path d="M0 {a} V{a + s} L{s} {2 * s} V{s}"/><path d="M{2 * s} {a} V{a + s} L{s} {2 * s}"/></g>')
    art = grid
    art += f'<ellipse cx="800" cy="360" rx="560" ry="250" fill="none" stroke="{V}" stroke-opacity="0.25" stroke-width="3" stroke-dasharray="2 12" stroke-linecap="round"/>'
    art += g(500, 170, headset)
    art += g(1060, 30, cube(58, P), False)
    art += g(270, 150, cube(56, C), False)
    art += g(260, 470, chip('AR · VR · XR', fill=V, color='#ffffff', size=30))
    art += g(1110, 470, chip('x 1.2  y 0.8', P, font=MONO, size=26))
    return page(V, P, art, defs)


# ---------- 4. Robotics ----------
def robo():
    O, G, T = '#f97316', '#22c55e', '#0f766e'
    defs = grad('rb', '#fb923c', '#ea580c', True)
    robot = ('<rect x="135" y="-66" width="10" height="66" fill="#94a3b8"/>'
             f'<circle cx="140" cy="-76" r="20" fill="{O}"/>'
             '<rect x="0" y="0" width="280" height="220" rx="58" fill="url(#rb)"/>'
             '<rect x="30" y="40" width="220" height="112" rx="42" fill="#1e293b"/>'
             f'<circle cx="95" cy="96" r="26" fill="{G}"/><circle cx="185" cy="96" r="26" fill="{G}"/>'
             '<circle cx="104" cy="87" r="8" fill="#ffffff"/><circle cx="194" cy="87" r="8" fill="#ffffff"/>'
             '<g fill="#ffffff" fill-opacity="0.85">' + ''.join(f'<rect x="{90 + i * 22}" y="172" width="14" height="22" rx="4"/>' for i in range(5)) + '</g>'
             '<rect x="-26" y="80" width="28" height="70" rx="10" fill="#cbd5e1"/><rect x="278" y="80" width="28" height="70" rx="10" fill="#cbd5e1"/>'
             '<rect x="40" y="240" width="200" height="150" rx="40" fill="url(#rb)"/>'
             '<rect x="80" y="270" width="120" height="66" rx="16" fill="#1e293b"/>'
             f'<circle cx="110" cy="303" r="10" fill="{G}"/><circle cx="140" cy="303" r="10" fill="#facc15"/><circle cx="170" cy="303" r="10" fill="#38bdf8"/>'
             '<rect x="-40" y="258" width="80" height="34" rx="17" fill="#cbd5e1" transform="rotate(-28 0 275)"/>'
             '<rect x="240" y="258" width="80" height="34" rx="17" fill="#cbd5e1" transform="rotate(28 280 275)"/>')
    board = (f'<rect width="420" height="260" rx="24" fill="{T}"/>'
             '<g fill="#0b2b2e">' + ''.join(f'<rect x="{40 + i * 25}" y="18" width="14" height="22" rx="2"/>' for i in range(14))
             + ''.join(f'<rect x="{115 + i * 25}" y="220" width="14" height="22" rx="2"/>' for i in range(10)) + '</g>'
             '<rect x="160" y="92" width="150" height="74" rx="8" fill="#14202a"/>'
             + t(235, 138, 'MCU', '#7fd7dc', 28, MONO, 700, 'middle')
             + '<rect x="22" y="82" width="74" height="58" rx="8" fill="#cbd5e1"/><rect x="22" y="156" width="58" height="46" rx="8" fill="#1b1b1b"/>'
             f'<circle cx="360" cy="84" r="9" fill="{G}"/><circle cx="360" cy="114" r="9" fill="#facc15"/>'
             '<g stroke="#7fd7dc" stroke-width="4" fill="none" stroke-opacity="0.8"><path d="M310 128 H370 V200"/><path d="M160 128 H120 V210"/></g>')
    art = dashed(['M690 420 C 760 420, 800 380, 860 370'], O)
    art += g(400, 190, robot)
    art += g(860, 240, board)
    art += g(1170, 140, gear(O), False)
    art += g(1050, 150, gear('#94a3b8'), False, ' scale(0.5)')
    art += g(250, 140, chip('digitalWrite', font=MONO, fill='#1e293b', color=G, size=24))
    art += g(1010, 560, chip('servo 90°', O, size=26))
    return page(O, T, art, defs)


# ---------- 5. Cybersecurity ----------
def cyber():
    S, R, G = '#0ea5e9', '#ef4444', '#22c55e'
    rnd = random.Random(7)
    binary = f'<g font-family="{MONO}" font-size="22" fill="{S}" fill-opacity="0.12">'
    for col in range(22):
        for row in range(14):
            if rnd.random() < 0.45:
                binary += f'<text x="{40 + col * 72}" y="{60 + row * 70}">{rnd.choice(["01", "10", "11", "00"])}</text>'
    binary += '</g>'
    L = lambda y, s, c='#e9ecff': t(40, y, s, c, 25, MONO, 500)
    term = darkwin(660, 410, L(104, '<tspan fill="#7ca0ff">$</tspan> nmap -sV 10.0.0.5')
                   + L(146, '22/tcp   open   ssh', '#4ade80') + L(182, '80/tcp   open   http', '#4ade80')
                   + L(218, '445/tcp  open   smb', '#fbbf24')
                   + L(270, '[!] найдена уязвимость SMBv1', '#f87171')
                   + L(312, '[+] патч установлен, порт закрыт', '#4ade80')
                   + L(364, '<tspan fill="#7ca0ff">$</tspan> ') + '<rect x="78" y="344" width="15" height="28" fill="#e9ecff"/>')
    target = (f'<circle r="120" fill="#ffffff" fill-opacity="0.6" stroke="{R}" stroke-width="8"/>'
              f'<circle r="76" fill="none" stroke="{R}" stroke-width="8" stroke-opacity="0.75"/>'
              f'<circle r="30" fill="{R}"/>'
              f'<g stroke="{R}" stroke-width="8" stroke-linecap="round"><path d="M0 -160 V-128 M0 128 V160 M-160 0 H-128 M128 0 H160"/></g>')
    lock = (f'<circle r="92" fill="{S}"/>'
            '<path d="M-30 -10 V-34 a30 30 0 0 1 60 0 V-10" fill="none" stroke="#ffffff" stroke-width="13" stroke-linecap="round"/>'
            '<rect x="-48" y="-14" width="96" height="74" rx="14" fill="#ffffff"/>'
            f'<circle cx="0" cy="18" r="10" fill="{S}"/><rect x="-4" y="20" width="8" height="24" rx="4" fill="{S}"/>')
    art = binary
    art += g(470, 150, term)
    art += g(1220, 350, target, extra=' scale(0.9)')
    art += g(420, 210, lock)
    art += g(1000, 540, chip('защищено', G, 280))
    art += g(250, 520, chip('фишинг?', R, 230))
    return page(S, R, art)


# ---------- 6. Internet marketing ----------
def marketing():
    R, A = '#ef4444', '#f59e0b'
    tiers = [(-270, 0, 540, '#fee2e2', '#991b1b'), (-220, 108, 440, '#fca5a5', '#7f1d1d'),
             (-170, 216, 340, '#f87171', '#ffffff'), (-120, 324, 240, R, '#ffffff')]
    funnel = ''
    for i, (x, y, w, col, tc) in enumerate(tiers):
        funnel += f'<path d="M{x} {y} h{w} l-30 94 h{-(w - 60)} z" fill="{col}"/>'
        lbl, n = [('показы', '120 000'), ('клики', '4 800'), ('заявки', '640'), ('продажи', '96')][i]
        funnel += t(0, y + 58, f'{lbl} · {n}', tc, 28, SANS, 800, 'middle')
    chart = card(380, 260, t(30, 54, 'ROMI', INK, 26, SANS, 800) + t(350, 54, '+212%', '#16a34a', 28, SANS, 800, 'end')
                 + ''.join(f'<rect x="{40 + i * 58}" y="{232 - h}" width="36" height="{h}" rx="8" fill="{R}" fill-opacity="{0.3 + i * 0.13}"/>'
                           for i, h in enumerate([40, 70, 60, 100, 130, 150]))
                 + '<path d="M58 176 L116 146 L174 156 L232 116 L290 86 L348 70" fill="none" stroke="#16a34a" stroke-width="6" '
                   'stroke-linecap="round" stroke-linejoin="round"/>')
    mega = (f'<path d="M0 30 L120 -24 V150 L0 96 Z" fill="{R}"/><rect x="-54" y="26" width="64" height="74" rx="16" fill="#1e293b"/>'
            '<rect x="-34" y="96" width="32" height="70" rx="12" fill="#1e293b"/>'
            f'<g stroke="{A}" stroke-width="10" stroke-linecap="round"><path d="M160 0 L205 -30"/><path d="M165 64 H222"/><path d="M160 128 L205 158"/></g>')
    art = g(770, 130, funnel)
    art += g(1050, 210, chart, extra=' scale(0.8)')
    art += g(310, 230, mega)
    art += g(250, 500, chip('CTR 4,2%', fill=INK, color='#fde68a', font=MONO, size=28))
    art += g(1120, 480, chip('utm_source', A, font=MONO, size=24))
    return page(R, A, art)


# ---------- 7. SMM ----------
def smm():
    P, V, Y = '#ec4899', '#8b5cf6', '#fbbf24'
    defs = grad('photo', '#f9a8d4', '#8b5cf6')
    phone = ('<rect width="340" height="640" rx="54" fill="#1e1b2e"/><rect x="14" y="14" width="312" height="612" rx="42" fill="#ffffff"/>'
             '<rect x="130" y="28" width="80" height="16" rx="8" fill="#1e1b2e"/>'
             f'<circle cx="62" cy="92" r="24" fill="{P}"/>' + bar(100, 80, 120, 12, INK, .75) + bar(100, 100, 80, 10, INK, .3)
             + '<rect x="34" y="134" width="272" height="272" rx="20" fill="url(#photo)"/>'
               '<circle cx="240" cy="200" r="32" fill="#ffffff" fill-opacity="0.85"/>'
               '<path d="M34 360 L124 274 L200 340 L248 302 L306 360 V386 a20 20 0 0 1 -20 20 H54 a20 20 0 0 1 -20 -20 Z" fill="#6d28d9" fill-opacity="0.55"/>'
             + g(66, 450, heart(0.85, P), False)
             + f'<path d="M116 434 h42 a10 10 0 0 1 10 10 v20 a10 10 0 0 1 -10 10 h-26 l-12 10 v-10 h-4 a10 10 0 0 1 -10 -10 v-20 a10 10 0 0 1 10 -10 z" fill="none" stroke="{INK}" stroke-width="5"/>'
             + bar(44, 508, 220, 12, INK, .5) + bar(44, 534, 250, 12, INK, .2) + bar(44, 560, 170, 12, INK, .2))
    bubble = card(300, 120, bar(32, 38, 210, 14, V, .75) + bar(32, 68, 150, 14, V, .35), 28) + \
        f'<path d="M44 119 l0 34 l34 -34 z" fill="#ffffff"/><path d="M44 119 l0 34 l34 -34" fill="none" stroke="{LINE}" stroke-width="1.5"/>'
    art = g(630, 110, phone)
    art += g(250, 170, chip('+1 250', P, size=32))
    art += g(1060, 400, bubble)
    art += g(1170, 270, heart(2.1, P), False)
    art += g(470, 470, heart(1.4, V), False)
    art += g(1110, 600, heart(1.0, Y), False)
    art += g(250, 520, chip('#контент', fill=V, color='#ffffff', size=28))
    return page(P, V, art, defs)


# ---------- 8. Data / SQL ----------
def data():
    B, C, G = '#2563eb', '#06b6d4', '#16a34a'
    defs = grad('dbg', '#60a5fa', '#1d4ed8', True)
    dash = card(580, 380, t(36, 60, 'Выручка по месяцам', INK, 28, SANS, 800) + t(544, 60, '+38%', G, 28, SANS, 800, 'end')
                + ''.join(f'<rect x="{52 + i * 64}" y="{336 - h}" width="40" height="{h}" rx="9" fill="{C if i % 2 else B}"/>'
                          for i, h in enumerate([90, 130, 110, 170, 150, 210, 240, 200]))
                + f'<path d="M72 236 L136 196 L200 211 L264 156 L328 171 L392 116 L456 86 L520 106" fill="none" stroke="{G}" '
                  'stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/>')
    db = ('<path d="M0 0 V210 a110 36 0 0 0 220 0 V0" fill="url(#dbg)"/>'
          '<path d="M0 70 a110 36 0 0 0 220 0 M0 140 a110 36 0 0 0 220 0" fill="none" stroke="#ffffff" stroke-opacity="0.5" stroke-width="4"/>'
          '<ellipse cx="110" cy="0" rx="110" ry="36" fill="#93c5fd"/>'
          '<circle cx="184" cy="46" r="7" fill="#4ade80"/><circle cx="184" cy="116" r="7" fill="#4ade80"/><circle cx="184" cy="186" r="7" fill="#4ade80"/>')
    sql = darkwin(470, 250, t(30, 98, '<tspan fill="#c792ea">SELECT</tspan> city, sum(total)', '#e9ecff', 23, MONO, 500)
                  + t(30, 134, '<tspan fill="#c792ea">FROM</tspan> orders', '#e9ecff', 23, MONO, 500)
                  + t(30, 170, '<tspan fill="#c792ea">GROUP BY</tspan> city', '#e9ecff', 23, MONO, 500)
                  + t(30, 206, '<tspan fill="#c792ea">ORDER BY</tspan> 2 <tspan fill="#c792ea">DESC</tspan>;', '#e9ecff', 23, MONO, 500))
    art = g(690, 140, dash, extra=' scale(0.88)')
    art += g(1185, 230, db, extra=' scale(0.78)')
    art += g(250, 270, sql, extra=' scale(0.92)')
    art += g(290, 140, chip('r = 0.87', G, font=MONO, size=26))
    return page(B, C, art, defs)


# ---------- 9. AI ----------
def ai():
    I, C, A = '#6366f1', '#06b6d4', '#f59e0b'
    defs = grad('chipg', '#818cf8', '#4338ca', True)
    layers = [(0, [110, 250, 390]), (210, [50, 180, 310, 440]), (420, [50, 180, 310, 440]), (630, [180, 320])]
    hot = {(0, 250), (210, 180), (420, 310), (630, 180)}
    net = f'<g stroke="{I}" stroke-opacity="0.2" stroke-width="3">'
    for (x1, ys1), (x2, ys2) in zip(layers, layers[1:]):
        for y1 in ys1:
            for y2 in ys2:
                net += f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}"/>'
    net += f'</g><g stroke="{C}" stroke-width="6" stroke-linecap="round">'
    net += '<line x1="0" y1="250" x2="210" y2="180"/><line x1="210" y1="180" x2="420" y2="310"/><line x1="420" y1="310" x2="630" y2="180"/></g>'
    for x, ys in layers:
        for y in ys:
            if (x, y) in hot:
                net += f'<circle cx="{x}" cy="{y}" r="46" fill="{C}" fill-opacity="0.2"/><circle cx="{x}" cy="{y}" r="30" fill="{C}"/>'
            else:
                net += f'<circle cx="{x}" cy="{y}" r="30" fill="#ffffff" stroke="{I}" stroke-width="6"/>'
    chip_ = ('<g stroke="#a5b4fc" stroke-width="10" stroke-linecap="round">'
             + ''.join(f'<line x1="{40 + i * 37}" y1="-26" x2="{40 + i * 37}" y2="0"/><line x1="{40 + i * 37}" y1="190" x2="{40 + i * 37}" y2="216"/>'
                       f'<line x1="-26" y1="{40 + i * 37}" x2="0" y2="{40 + i * 37}"/><line x1="190" y1="{40 + i * 37}" x2="216" y2="{40 + i * 37}"/>'
                       for i in range(4)) + '</g>'
             '<rect width="190" height="190" rx="32" fill="url(#chipg)"/>'
             '<rect x="26" y="26" width="138" height="138" rx="22" fill="none" stroke="#ffffff" stroke-opacity="0.4" stroke-width="3"/>'
             + t(95, 117, 'AI', '#ffffff', 60, SANS, 900, 'middle'))
    art = g(560, 140, net, False)
    art += g(260, 260, chip_)
    art += g(1170, 110, star(1.4, A), False)
    art += g(1300, 280, star(0.75, I), False)
    art += g(1110, 520, chip('промпт → ответ', I, size=26))
    art += g(240, 540, chip('accuracy 0.96', fill=INK, color='#67e8f9', font=MONO, size=24))
    return page(I, C, art, defs)


# ---------- 10. UX/UI ----------
def ux():
    V, O, S = '#8b5cf6', '#ff7a59', '#3b82f6'
    board = card(330, 560, '<rect x="22" y="28" width="286" height="46" rx="12" fill="#f3f4fa"/>' + bar(42, 45, 110, 12, V, .7)
                 + f'<rect x="22" y="94" width="286" height="166" rx="18" fill="{O}"/>'
                 + bar(46, 186, 180, 18, '#ffffff', .95) + bar(46, 218, 120, 12, '#ffffff', .75)
                 + ''.join(f'<rect x="{22 + c * 148}" y="{280 + r * 128}" width="138" height="114" rx="16" fill="#f3f4fa"/>'
                           f'<rect x="{38 + c * 148}" y="{296 + r * 128}" width="40" height="40" rx="10" fill="{col}"/>'
                           + bar(38 + c * 148, 352 + r * 128, 90, 10, INK, .3)
                           for r in range(2) for c, col in enumerate(['#22d3ee', V] if r == 0 else ['#4ade80', O])), 36)
    sel = (f'<rect x="-6" y="-6" width="298" height="178" fill="none" stroke="{S}" stroke-width="3"/>'
           + ''.join(f'<rect x="{x - 9}" y="{y - 9}" width="18" height="18" fill="#ffffff" stroke="{S}" stroke-width="3"/>'
                     for x, y in [(-6, -6), (292, -6), (-6, 172), (292, 172)])
           + f'<rect x="306" y="66" width="134" height="38" rx="8" fill="{S}"/>' + t(373, 93, '286 × 166', '#ffffff', 20, SANS, 700, 'middle'))
    pen = (f'<path d="M0 0 L60 -110 L120 0 L60 60 Z" fill="{INK}"/><circle cx="60" cy="-8" r="13" fill="#ffffff"/>'
           '<path d="M60 -110 V-22" stroke="#ffffff" stroke-width="6"/>'
           f'<rect x="30" y="66" width="60" height="70" rx="10" fill="{V}"/>')
    swatches = ''.join(f'<circle cx="{i * 78}" cy="0" r="30" fill="{c}" stroke="#ffffff" stroke-width="6"/>'
                       for i, c in enumerate([INK, V, '#22d3ee', '#4ade80', O, '#fbbf24']))
    typo = card(270, 230, t(30, 80, 'Aa', V, 72, SANS, 900) + bar(30, 116, 200, 14, INK, .65) + bar(30, 146, 160, 12, INK, .25)
                + bar(30, 172, 180, 12, INK, .25) + t(30, 212, 'Inter · 16/24', '#7b829e', 20, MONO, 600))
    art = g(640, 110, board)
    art += g(662, 204, sel, False)
    art += g(1200, 310, pen, extra=' rotate(20)')
    art += g(1120, 520, f'<rect x="-50" y="-48" width="490" height="96" rx="48" fill="#ffffff"/>'
             f'<rect x="-50" y="-48" width="490" height="96" rx="48" fill="none" stroke="{LINE}" stroke-width="1.5"/>' + swatches, True, ' scale(0.82)')
    art += g(260, 170, typo)
    art += g(260, 470, chip('auto layout', fill=INK, color='#93c5fd', font=MONO, size=26))
    return page(V, O, art)


# ---------- Системное администрирование ----------
def sysadmin():
    I, G, A = '#4f46e5', '#22c55e', '#f59e0b'
    defs = grad('rack', '#334155', '#1e293b', True)
    units = ''
    for i, (l1, l2) in enumerate([(G, G), (A, G), (G, G), (G, A)]):
        y = 30 + i * 108
        units += (f'<g transform="translate(24 {y})"><rect width="252" height="90" rx="14" fill="#475569"/>'
                  + bar(22, 26, 110, 12, '#94a3b8') + bar(22, 52, 70, 12, '#64748b')
                  + f'<circle cx="190" cy="45" r="9" fill="{l1}"/><circle cx="222" cy="45" r="9" fill="{l2}"/></g>')
    rack = ('<rect width="300" height="500" rx="28" fill="url(#rack)"/>' + units
            + '<g stroke="#475569" stroke-width="5" stroke-linecap="round"><path d="M40 470 H260"/></g>')
    L = lambda y, s, c='#e9ecff': t(32, y, s, c, 24, MONO, 500)
    term = darkwin(430, 270, L(100, '<tspan fill="#7ca0ff">$</tspan> ping 8.8.8.8') + L(136, '64 bytes: time=12 ms', '#4ade80')
                   + L(184, '<tspan fill="#7ca0ff">$</tspan> systemctl status') + L(220, '● active (running)', '#4ade80'))
    laptop = card(200, 130, f'<rect x="14" y="14" width="172" height="102" rx="10" fill="#eef0fb"/>' + bar(32, 34, 80, 12, I)
                  + bar(32, 56, 120, 12, I, .35) + bar(32, 78, 96, 12, I, .35), 16) \
        + '<path d="M-22 130 h244 l-16 24 h-212 z" fill="#cbd5e1"/>'
    router = (card(200, 70, f'<circle cx="46" cy="35" r="8" fill="{G}"/><circle cx="76" cy="35" r="8" fill="{G}"/>'
                   f'<circle cx="106" cy="35" r="8" fill="{A}"/><circle cx="136" cy="35" r="8" fill="{G}"/>', 18)
              + '<rect x="30" y="-70" width="12" height="74" rx="6" fill="#cbd5e1"/><rect x="158" y="-70" width="12" height="74" rx="6" fill="#cbd5e1"/>'
              + f'<g fill="none" stroke="{I}" stroke-width="8" stroke-linecap="round" transform="translate(100 -40)">'
                '<path d="M-26 -6 a38 38 0 0 1 52 0"/><path d="M-48 -28 a68 68 0 0 1 96 0" stroke-opacity="0.5"/></g>')
    shieldok = (f'<path d="M0 -62 L52 -42 V4 C52 40 28 64 0 76 C-28 64 -52 40 -52 4 V-42 Z" fill="{G}"/>'
                '<path d="M-20 4 l14 14 l28 -30" fill="none" stroke="#ffffff" stroke-width="10" stroke-linecap="round" stroke-linejoin="round"/>')
    art = dashed(['M490 260 C 520 260, 540 250, 560 240', 'M1170 400 C 1180 450, 1180 480, 1170 520'], I)
    art += g(270, 170, laptop)
    art += g(560, 110, rack)
    art += g(740, 280, term)
    art += g(1170, 330, router)
    art += g(1110, 170, gear(A), False, ' scale(0.9)')
    art += g(560, 110, shieldok, extra=' scale(0.85)')
    art += g(260, 470, chip('sudo apt update', fill=INK, color='#a5b4fc', font=MONO, size=24))
    return page(I, G, art, defs)


# ---------- Цифровая гигиена ----------
def gigiena():
    G, S, R = '#10b981', '#0ea5e9', '#ef4444'
    defs = grad('sh', '#34d399', '#047857', True)
    shield = ('<path d="M0 -240 L200 -168 V10 C200 156 104 250 0 300 C-104 250 -200 156 -200 10 V-168 Z" fill="url(#sh)"/>'
              '<path d="M0 -206 L166 -146 V10 C166 136 86 216 0 258 C-86 216 -166 136 -166 10 V-146 Z" fill="none" stroke="#ffffff" stroke-opacity="0.35" stroke-width="4"/>'
              '<path d="M-50 -20 V-60 a50 50 0 0 1 100 0 V-20" fill="none" stroke="#ffffff" stroke-width="22" stroke-linecap="round"/>'
              '<rect x="-84" y="-24" width="168" height="132" rx="24" fill="#ffffff"/>'
              '<circle cx="0" cy="30" r="17" fill="#047857"/><rect x="-7" y="32" width="14" height="42" rx="7" fill="#047857"/>')
    phone = ('<rect width="210" height="370" rx="36" fill="#1e293b"/><rect x="12" y="12" width="186" height="346" rx="26" fill="#ffffff"/>'
             f'<circle cx="105" cy="92" r="34" fill="{G}" fill-opacity="0.15"/>'
             f'<path d="M89 92 l11 11 l22 -24" fill="none" stroke="{G}" stroke-width="8" stroke-linecap="round" stroke-linejoin="round"/>'
             + t(105, 180, '482 913', INK, 38, SANS, 900, 'middle') + bar(40, 206, 130, 10, INK, .2)
             + f'<rect x="40" y="250" width="130" height="44" rx="22" fill="{G}"/>' + t(105, 280, 'Войти', '#ffffff', 20, SANS, 800, 'middle'))
    pwd = card(320, 150, t(28, 46, 'Пароль', '#7b829e', 20, SANS, 700)
               + '<rect x="28" y="62" width="264" height="44" rx="12" fill="#f3f4fa"/>'
               + ''.join(f'<circle cx="{52 + i * 26}" cy="84" r="7" fill="{INK}"/>' for i in range(9))
               + ''.join(f'<rect x="{28 + i * 68}" y="122" width="60" height="8" rx="4" fill="{G}"/>' for i in range(4)))
    mail = (card(220, 150, f'<path d="M14 20 L110 90 L206 20" fill="none" stroke="#cbd5e1" stroke-width="6" stroke-linejoin="round"/>', 18)
            + f'<circle cx="214" cy="6" r="30" fill="{R}"/><path d="M202 -6 l24 24 M226 -6 l-24 24" stroke="#ffffff" stroke-width="7" stroke-linecap="round"/>')
    art = dashed(['M520 300 C 560 300, 580 290, 600 280', 'M1000 300 C 1010 300, 1015 290, 1020 280'], G)
    art += g(300, 130, phone)
    art += g(800, 330, shield)
    art += g(1020, 190, pwd)
    art += g(1100, 420, mail)
    art += g(260, 530, chip('2FA включена', G, size=26))
    return page(G, S, art, defs)


COVERS = {
    'sysadmin-basics': sysadmin, 'cifrovaya-gigiena': gigiena,
    'python-s-nulya': python, 'web-razrabotka': web, 'vr-ar-razrabotka': vr, 'robototehnika-arduino': robo,
    'kiberbezopasnost': cyber, 'internet-marketolog': marketing, 'smm-specialist': smm,
    'analiz-dannyh-sql': data, 'neyroseti-ii': ai, 'ux-ui-dizajn': ux,
}

if __name__ == '__main__':
    for slug, fn in COVERS.items():
        with open(os.path.join(OUT, slug + '.html'), 'w', encoding='utf-8') as f:
            f.write(fn())
        print('ok', slug)
