"""Generates the neon harbour background for K8s Dockside as SVG.

A harbour in the wireframe-and-glow idiom: a container ship of glowing boxes
at anchor, gantry cranes on a pier, the seven-spoked helm as a moon, and a mesh
of lit nodes lying on the water like a constellation of pods. Everything is
vector so it stays crisp at any window size and weighs little.

    python3 neon_harbour.py k8s_dockside_neon_harbour.svg
    python3 neon_harbour.py --light k8s_dockside_neon_harbour_light.svg

The night version is for the dark themes. The light themes get the same scene
by day: the same lines in deeper inks, on a pale sky and sea, because a dark
picture faded over a light window only ever reads as grey.
"""
import math
import random
import sys

LIGHT = '--light' in sys.argv
OUT = [a for a in sys.argv[1:] if not a.startswith('--')][0]
random.seed(20260907)
W, H = 1920, 1080
HZ = 690  # the horizon: the welcome text sits in the sky above it

if LIGHT:
    BLUE, CYAN, PINK, PURPLE, WARM, WHITE = '#2f6cf0', '#0aa6c9', '#e0338f', '#7b4de6', '#f59e0b', '#3b4a7a'
    INK, INK2 = '#f7f8fe', '#e3e8f7'   # what a hull or a shed is filled with
    SKY = ('#f3f5ff', '#eef0ff', '#f7e6f6', '#ffe0ef')
    SEA = ('#f3e3f2', '#e6ebfb', '#cfd8ef')
    VIGNETTE, VIG_A = '#3b4a7a', 0.22
    STARS = 90
else:
    BLUE, CYAN, PINK, PURPLE, WARM, WHITE = '#4f8cff', '#3ee6ff', '#ff3fa4', '#9b6bff', '#ffb347', '#eef2ff'
    INK, INK2 = '#07051a', '#0b0720'
    SKY = ('#03020b', '#0c0726', '#22103f', '#3a1553')
    SEA = ('#1a0f3a', '#0c0a2a', '#03020c')
    VIGNETTE, VIG_A = '#000000', 0.6
    STARS = 300
DECK, KEEL = 750, 842  # the ship's deck line and its keel, on the water

out = []
def add(s): out.append(s)

def hexa(color, a):
    """A colour with an alpha, as rgba(), so opacity is per element."""
    r, g, b = int(color[1:3], 16), int(color[3:5], 16), int(color[5:7], 16)
    return f'rgba({r},{g},{b},{a:.3f})'

# ---------------------------------------------------------------- defs
add(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}" role="img" aria-label="A neon harbour at night: a container ship, gantry cranes and a mesh of lights on the water">')
add('<title>K8s Dockside — neon harbour</title>')
add('<defs>')
add(f'''<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
  <stop offset="0" stop-color="{SKY[0]}"/><stop offset="0.45" stop-color="{SKY[1]}"/>
  <stop offset="0.8" stop-color="{SKY[2]}"/><stop offset="1" stop-color="{SKY[3]}"/></linearGradient>
<linearGradient id="sea" x1="0" y1="0" x2="0" y2="1">
  <stop offset="0" stop-color="{SEA[0]}"/><stop offset="0.25" stop-color="{SEA[1]}"/>
  <stop offset="1" stop-color="{SEA[2]}"/></linearGradient>
<radialGradient id="dawn" cx="0.36" cy="1" r="0.55">
  <stop offset="0" stop-color="{PINK}" stop-opacity="0.55"/><stop offset="0.45" stop-color="#b0308f" stop-opacity="0.22"/>
  <stop offset="1" stop-color="{PINK}" stop-opacity="0"/></radialGradient>
<radialGradient id="nebula" cx="0.78" cy="0.22" r="0.42">
  <stop offset="0" stop-color="{BLUE}" stop-opacity="0.38"/><stop offset="0.5" stop-color="{PURPLE}" stop-opacity="0.14"/>
  <stop offset="1" stop-color="{BLUE}" stop-opacity="0"/></radialGradient>
<radialGradient id="moonglow" cx="0.5" cy="0.5" r="0.5">
  <stop offset="0" stop-color="{CYAN}" stop-opacity="0.35"/><stop offset="0.35" stop-color="{BLUE}" stop-opacity="0.16"/>
  <stop offset="1" stop-color="{PURPLE}" stop-opacity="0"/></radialGradient>
<radialGradient id="vignette" cx="0.5" cy="0.55" r="0.75">
  <stop offset="0.55" stop-color="{VIGNETTE}" stop-opacity="0"/><stop offset="1" stop-color="{VIGNETTE}" stop-opacity="{VIG_A}"/></radialGradient>
<linearGradient id="streak" x1="0" y1="0" x2="1" y2="0">
  <stop offset="0" stop-color="{BLUE}" stop-opacity="0"/><stop offset="0.35" stop-color="{BLUE}" stop-opacity="0.9"/>
  <stop offset="0.7" stop-color="{PINK}" stop-opacity="0.8"/><stop offset="1" stop-color="{PINK}" stop-opacity="0"/></linearGradient>
<linearGradient id="fade-down" x1="0" y1="0" x2="0" y2="1">
  <stop offset="0" stop-color="#fff" stop-opacity="1"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></linearGradient>
<filter id="glow" x="-30%" y="-30%" width="160%" height="160%">
  <feGaussianBlur stdDeviation="4" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
<filter id="glow-soft" x="-30%" y="-30%" width="160%" height="160%">
  <feGaussianBlur stdDeviation="9" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
<filter id="blur-wide" x="-40%" y="-40%" width="180%" height="180%"><feGaussianBlur stdDeviation="26"/></filter>
<filter id="blur-soft" x="-40%" y="-40%" width="180%" height="180%"><feGaussianBlur stdDeviation="5"/></filter>
<clipPath id="below-horizon"><rect x="0" y="{HZ}" width="{W}" height="{H-HZ}"/></clipPath>
<clipPath id="above-horizon"><rect x="0" y="0" width="{W}" height="{HZ}"/></clipPath>''')
add('</defs>')

# ---------------------------------------------------------------- sky
add(f'<rect width="{W}" height="{HZ+2}" fill="url(#sky)"/>')
add(f'<rect width="{W}" height="{HZ+2}" fill="url(#nebula)"/>')
add(f'<rect width="{W}" height="{HZ+2}" fill="url(#dawn)"/>')

# stars
add('<g>')
# Every star is drawn from the same random stream in both versions, so the
# two files are one scene by night and by day; the day sky just keeps fewer.
for i in range(300):
    x, y = random.uniform(0, W), random.uniform(0, HZ - 30)
    r = random.choice([0.6, 0.8, 1.0, 1.2, 1.6, 2.0])
    a = random.uniform(0.25, 1.0) * (1 - y / HZ * 0.5)
    c = random.choice([WHITE, WHITE, WHITE, BLUE, PINK])
    if i % (300 // STARS) == 0:
        add(f'<circle cx="{x:.0f}" cy="{y:.0f}" r="{r}" fill="{hexa(c, a)}"/>')
add('</g>')
add('<g filter="url(#glow)">')
for _ in range(16):
    x, y = random.uniform(40, W - 40), random.uniform(20, HZ - 120)
    add(f'<circle cx="{x:.0f}" cy="{y:.0f}" r="{random.uniform(1.6, 2.6):.1f}" fill="{random.choice([WHITE, CYAN, PINK])}"/>')
add('</g>')

# aurora streaks: a bezier band, blurred, with particles riding it
def bezier(p0, p1, p2, p3, t):
    u = 1 - t
    return (u**3*p0[0] + 3*u*u*t*p1[0] + 3*u*t*t*p2[0] + t**3*p3[0],
            u**3*p0[1] + 3*u*u*t*p1[1] + 3*u*t*t*p2[1] + t**3*p3[1])

streaks = [
    ((-60, 150), (420, 40), (700, 330), (1180, 420)),
    ((-40, 260), (380, 150), (760, 420), (1250, 470)),
    ((900, 60), (1300, 20), (1500, 200), (1900, 380)),
]
add('<g clip-path="url(#above-horizon)">')
for i, (p0, p1, p2, p3) in enumerate(streaks):
    d = f'M{p0[0]},{p0[1]} C{p1[0]},{p1[1]} {p2[0]},{p2[1]} {p3[0]},{p3[1]}'
    add(f'<path d="{d}" fill="none" stroke="url(#streak)" stroke-width="{[70, 40, 55][i]}" stroke-linecap="round" opacity="0.32" filter="url(#blur-wide)"/>')
    add(f'<path d="{d}" fill="none" stroke="url(#streak)" stroke-width="2" opacity="0.5" filter="url(#blur-soft)"/>')
    add('<g>')
    for _ in range(170):
        t = random.random()
        x, y = bezier(p0, p1, p2, p3, t)
        spread = 26 + 30 * math.sin(t * math.pi)
        x += random.gauss(0, spread); y += random.gauss(0, spread * 0.55)
        a = random.uniform(0.2, 0.95)
        c = random.choice([BLUE, CYAN, PINK, WHITE, '#b7c8ff'])
        add(f'<circle cx="{x:.0f}" cy="{y:.0f}" r="{random.uniform(0.7, 1.9):.1f}" fill="{hexa(c, a)}"/>')
    add('</g>')
add('</g>')

# ---------------------------------------------------------------- the helm as a moon
MX, MY, MR = 1000, 245, 125
add(f'<circle cx="{MX}" cy="{MY}" r="{MR*2.1:.0f}" fill="url(#moonglow)"/>')
def heptagon(r, rot=-90):
    pts = []
    for k in range(7):
        a = math.radians(rot + k * 360 / 7)
        pts.append((MX + r * math.cos(a), MY + r * math.sin(a)))
    return pts
outer, inner = heptagon(MR), heptagon(MR * 0.62)
def poly(pts):
    return ' '.join(f'{x:.1f},{y:.1f}' for x, y in pts)
add('<g filter="url(#glow)" stroke-linejoin="round" stroke-linecap="round" fill="none">')
add(f'<polygon points="{poly(outer)}" stroke="{CYAN}" stroke-width="2.4" opacity="0.85"/>')
add(f'<polygon points="{poly(inner)}" stroke="{BLUE}" stroke-width="1.6" opacity="0.7"/>')
for (ox, oy), (ix, iy) in zip(outer, inner):
    add(f'<line x1="{MX}" y1="{MY}" x2="{ox:.1f}" y2="{oy:.1f}" stroke="{CYAN}" stroke-width="1.6" opacity="0.75"/>')
    add(f'<circle cx="{ox:.1f}" cy="{oy:.1f}" r="4" fill="{WHITE}" stroke="none"/>')
add(f'<circle cx="{MX}" cy="{MY}" r="{MR*0.16:.0f}" fill="{hexa(CYAN,0.35)}" stroke="{CYAN}" stroke-width="2"/>')
add('</g>')
# a faint outer ring for the moon's body
add(f'<circle cx="{MX}" cy="{MY}" r="{MR*1.18:.0f}" fill="none" stroke="{hexa(PURPLE,0.35)}" stroke-width="1" stroke-dasharray="3 9"/>')

# ---------------------------------------------------------------- distant port on the horizon
add('<g>')
x = -20
while x < W:
    w = random.uniform(30, 110); h = random.uniform(6, 34)
    if 380 < x < 1300 and random.random() < 0.5:
        x += w + random.uniform(0, 60); continue
    add(f'<rect x="{x:.0f}" y="{HZ-h:.0f}" width="{w:.0f}" height="{h:.0f}" fill="{INK2}" stroke="{hexa(PURPLE,0.45)}" stroke-width="0.8"/>')
    for _ in range(int(w // 12)):
        if random.random() < 0.55:
            add(f'<rect x="{random.uniform(x+3, x+w-4):.0f}" y="{random.uniform(HZ-h+3, HZ-3):.0f}" width="1.6" height="1.6" fill="{hexa(random.choice([WARM, CYAN, PINK, WHITE]), random.uniform(0.4, 1))}"/>')
    x += w + random.uniform(0, 40)
# far cranes as tiny glyphs
for cx in (60, 200, 1580, 1720, 1860):
    add(f'<path d="M{cx},{HZ} v-46 h-40 M{cx-6},{HZ} v-46 M{cx-40},{HZ-46} l-14,10" fill="none" stroke="{hexa(PINK,0.55)}" stroke-width="1.2"/>')
add('</g>')
add(f'<line x1="0" y1="{HZ}" x2="{W}" y2="{HZ}" stroke="{hexa(PINK,0.6)}" stroke-width="1.2" filter="url(#glow-soft)"/>')

# ---------------------------------------------------------------- water
add(f'<rect x="0" y="{HZ}" width="{W}" height="{H-HZ}" fill="url(#sea)"/>')
# the horizon glow mirrored into the water
add(f'<rect x="0" y="{HZ}" width="{W}" height="220" fill="url(#dawn)" opacity="0.55" transform="translate(0,{2*HZ+220}) scale(1,-1)"/>')

# the mesh of nodes on the water: denser and finer near the horizon
nodes = []
for _ in range(260):
    t = random.random() ** 0.55  # more of them far away
    y = HZ + 14 + t * (H - HZ - 30)
    nodes.append((random.uniform(-40, W + 40), y))
add('<g clip-path="url(#below-horizon)">')
add('<g stroke-linecap="round">')
for i, (x1, y1) in enumerate(nodes):
    reach = 40 + (y1 - HZ) * 0.42
    links = 0
    for j in range(i + 1, len(nodes)):
        x2, y2 = nodes[j]
        d = math.hypot(x2 - x1, (y2 - y1) * 1.6)
        if d < reach:
            a = 0.08 + 0.3 * (1 - d / reach)
            c = random.choice([BLUE, PURPLE, PURPLE, PINK])
            add(f'<line x1="{x1:.0f}" y1="{y1:.0f}" x2="{x2:.0f}" y2="{y2:.0f}" stroke="{hexa(c, a)}" stroke-width="{0.7 + (y1-HZ)/H*1.4:.1f}"/>')
            links += 1
            if links > 4: break
add('</g><g>')
for (x, y) in nodes:
    depth = (y - HZ) / (H - HZ)
    r = 0.8 + depth * 2.6
    c = random.choice([BLUE, CYAN, PINK, WHITE, PURPLE])
    add(f'<circle cx="{x:.0f}" cy="{y:.0f}" r="{r:.1f}" fill="{hexa(c, random.uniform(0.35, 0.95))}"/>')
add('</g><g filter="url(#glow)">')
for (x, y) in random.sample(nodes, 14):
    depth = (y - HZ) / (H - HZ)
    add(f'<circle cx="{x:.0f}" cy="{y:.0f}" r="{2 + depth*3:.1f}" fill="{random.choice([CYAN, PINK, WHITE])}"/>')
add('</g></g>')

# the moon's reflection, laid down before the ship so the hull sits on it
def reflect(x, y_top, color, length, width=3, alpha=0.5):
    """A broken vertical streak under a light, the way water carries one."""
    y = y_top
    while y < min(y_top + length, H):
        seg = random.uniform(6, 22); gap = random.uniform(5, 16)
        a = alpha * (1 - (y - y_top) / length)
        add(f'<line x1="{x + random.uniform(-3,3):.0f}" y1="{y:.0f}" x2="{x + random.uniform(-3,3):.0f}" y2="{y+seg:.0f}" stroke="{hexa(color, a)}" stroke-width="{width}" stroke-linecap="round"/>')
        y += seg + gap
add('<g clip-path="url(#below-horizon)">')
reflect(MX, KEEL + 6, CYAN, 300, 8, 0.4)
reflect(MX + 18, KEEL + 6, BLUE, 240, 4, 0.3)
reflect(MX - 22, KEEL + 6, PINK, 200, 3, 0.25)
add('</g>')

# ---------------------------------------------------------------- the ship
BOW, STERN = 380, 1270
hull = [(BOW, DECK), (STERN, DECK), (STERN-14, KEEL-30), (STERN-50, KEEL), (BOW+150, KEEL), (BOW+60, KEEL-32)]
add('<g>')
add(f'<polygon points="{poly(hull)}" fill="{INK}" fill-opacity="0.92"/>')
# ribs and waterlines, as the wireframe of the hull
add(f'<g stroke="{hexa(CYAN,0.22)}" stroke-width="1">')
for rx in range(BOW + 90, STERN - 40, 44):
    add(f'<line x1="{rx}" y1="{DECK}" x2="{rx}" y2="{KEEL}"/>')
for wy in (DECK + 30, DECK + 60):
    add(f'<line x1="{BOW + (wy-DECK)*0.9:.0f}" y1="{wy}" x2="{STERN-8}" y2="{wy}"/>')
add('</g>')
add('<g filter="url(#glow)" fill="none" stroke-linejoin="round">')
add(f'<polygon points="{poly(hull)}" stroke="{CYAN}" stroke-width="2.6"/>')
add(f'<line x1="{BOW+22}" y1="{DECK+8}" x2="{STERN-12}" y2="{DECK+8}" stroke="{hexa(PINK,0.8)}" stroke-width="1.4"/>')
add('</g>')
# superstructure at the stern
SX, SY = 1120, DECK - 140
add('<g filter="url(#glow)" fill="none" stroke-linejoin="round">')
add(f'<rect x="{SX}" y="{SY+40}" width="110" height="{DECK-SY-40}" fill="{INK}" fill-opacity="0.9" stroke="{BLUE}" stroke-width="2"/>')
add(f'<rect x="{SX+12}" y="{SY}" width="86" height="42" fill="{INK}" fill-opacity="0.9" stroke="{BLUE}" stroke-width="2"/>')
add(f'<rect x="{SX+30}" y="{SY-40}" width="16" height="40" stroke="{PINK}" stroke-width="1.8"/>')  # funnel
add(f'<line x1="{SX+80}" y1="{SY}" x2="{SX+80}" y2="{SY-90}" stroke="{CYAN}" stroke-width="1.6"/>')  # mast
add(f'<line x1="{SX+58}" y1="{SY-60}" x2="{SX+102}" y2="{SY-60}" stroke="{CYAN}" stroke-width="1.4"/>')
add(f'<circle cx="{SX+80}" cy="{SY-92}" r="3.2" fill="{PINK}" stroke="none"/>')
add('</g>')
# windows
add('<g>')
for row in range(4):
    for col in range(5):
        if random.random() < 0.8:
            add(f'<rect x="{SX+14+col*20}" y="{SY+52+row*26}" width="10" height="6" fill="{hexa(random.choice([WARM, WARM, CYAN, WHITE]), random.uniform(0.5,1))}"/>')
for col in range(5):
    add(f'<rect x="{SX+22+col*14}" y="{SY+14}" width="9" height="7" fill="{hexa(CYAN, random.uniform(0.5,1))}"/>')
add('</g>')

# containers stacked on deck: every box a pod; a few lit brighter, one or two amiss
CW, CH, GAP = 54, 27, 5
cols = list(range(BOW + 130, SX - 30, CW + GAP))
heights = [max(1, min(4, round(4.2 - abs((i - len(cols)*0.55) / len(cols)) * 6 + random.uniform(-0.6, 0.8)))) for i in range(len(cols))]
add('<g stroke-linejoin="round">')
lit = []
for i, cx in enumerate(cols):
    for level in range(heights[i]):
        y = DECK - (level + 1) * (CH + GAP) + GAP
        c = random.choice([BLUE, BLUE, PINK, CYAN, PURPLE, PURPLE])
        bright = random.random() < 0.18
        fill_a = 0.55 if bright else 0.16
        add(f'<rect x="{cx}" y="{y}" width="{CW}" height="{CH}" rx="2" fill="{hexa(c, fill_a)}" stroke="{c}" stroke-width="{1.8 if bright else 1.2}" stroke-opacity="{0.95 if bright else 0.7}"/>')
        # corrugation lines
        for k in range(1, 4):
            add(f'<line x1="{cx + k*CW/4:.0f}" y1="{y+4}" x2="{cx + k*CW/4:.0f}" y2="{y+CH-4}" stroke="{hexa(c, 0.35)}" stroke-width="0.8"/>')
        if bright: lit.append((cx, y, c))
add('</g>')
add('<g filter="url(#glow-soft)">')
for cx, y, c in lit:
    add(f'<rect x="{cx}" y="{y}" width="{CW}" height="{CH}" rx="2" fill="none" stroke="{c}" stroke-width="1.6" opacity="0.9"/>')
add('</g>')
# the bow light and anchor chain
add(f'<g filter="url(#glow)"><circle cx="{BOW+18}" cy="{DECK-12}" r="3.4" fill="{CYAN}"/><line x1="{BOW+18}" y1="{DECK}" x2="{BOW+18}" y2="{DECK-12}" stroke="{CYAN}" stroke-width="1.4"/></g>')
add(f'<path d="M{BOW+40},{DECK+6} c-30,30 -60,60 -70,120" fill="none" stroke="{hexa(WHITE,0.35)}" stroke-width="1.2" stroke-dasharray="4 5"/>')
add('</g>')

# ---------------------------------------------------------------- the quay and its cranes
QX = 1420
add('<g>')
add(f'<rect x="{QX}" y="{DECK-8}" width="{W-QX}" height="18" fill="{INK}" stroke="{PINK}" stroke-width="1.8" filter="url(#glow)"/>')
add(f'<line x1="{QX+6}" y1="{DECK+1}" x2="{W}" y2="{DECK+1}" stroke="{hexa(PINK,0.35)}" stroke-width="1"/>')
# pilings, braced, standing in the water
pil = list(range(QX + 24, W + 60, 118))
add(f'<g stroke="{hexa(PINK,0.6)}" stroke-width="3" stroke-linecap="round">')
for px in pil:
    add(f'<line x1="{px}" y1="{DECK+10}" x2="{px}" y2="{H}"/>')
add('</g>')
add(f'<g stroke="{hexa(PURPLE,0.35)}" stroke-width="1.2">')
for a, b in zip(pil, pil[1:]):
    for y0 in (DECK + 40, DECK + 150):
        add(f'<path d="M{a},{y0} L{b},{y0+90} M{b},{y0} L{a},{y0+90}"/>')
    add(f'<line x1="{a}" y1="{DECK+120}" x2="{b}" y2="{DECK+120}"/>')
add('</g>')
# a few boxes waiting on the pier
for i, c in enumerate([BLUE, PINK, CYAN]):
    bx = QX + 620 + i * 58; by = DECK - 8 - 27
    add(f'<rect x="{bx}" y="{by}" width="54" height="27" rx="2" fill="{hexa(c,0.2)}" stroke="{c}" stroke-width="1.2" stroke-opacity="0.8"/>')
add(f'<rect x="{QX+678}" y="{DECK-8-54}" width="54" height="27" rx="2" fill="{hexa(PURPLE,0.2)}" stroke="{PURPLE}" stroke-width="1.2" stroke-opacity="0.8"/>')
# bollards
for bx in (QX + 70, QX + 250, QX + 440):
    add(f'<rect x="{bx}" y="{DECK-24}" width="14" height="20" rx="3" fill="{INK}" stroke="{hexa(CYAN,0.8)}" stroke-width="1.4"/>')
add(f'<path d="M{STERN-30},{DECK+10} C{STERN+60},{DECK+70} {QX+40},{DECK+40} {QX+77},{DECK-14}" fill="none" stroke="{hexa(WHITE,0.4)}" stroke-width="1.2"/>')
add('</g>')

def crane(x, scale, hue, boom_left=True):
    s = scale
    g = DECK - 6
    legs = 90 * s; top = g - 300 * s; beam = top + 26 * s
    add(f'<g filter="url(#glow)" fill="none" stroke="{hue}" stroke-linejoin="round" stroke-linecap="round">')
    # legs with X bracing
    add(f'<path d="M{x},{g} V{beam} M{x+legs},{g} V{beam}" stroke-width="{2.6*s:.1f}"/>')
    for k in range(3):
        y0 = g - k * 90 * s; y1 = y0 - 90 * s
        if y1 < beam: y1 = beam
        add(f'<path d="M{x},{y0} L{x+legs},{y1} M{x+legs},{y0} L{x},{y1}" stroke-width="{1.1*s:.1f}" opacity="0.6"/>')
    # the beam and boom
    bx0 = x - 320 * s if boom_left else x
    bx1 = x + legs + 40 * s
    add(f'<path d="M{bx0},{beam} H{bx1}" stroke-width="{2.8*s:.1f}"/>')
    add(f'<path d="M{bx0},{beam-10*s} H{bx1} M{bx0},{beam-10*s} V{beam} M{bx1},{beam-10*s} V{beam}" stroke-width="{1.2*s:.1f}" opacity="0.7"/>')
    # A-frame and ties
    ax = x + legs * 0.5; ay = top - 70 * s
    add(f'<path d="M{x+10*s},{beam-10*s} L{ax},{ay} L{x+legs-10*s},{beam-10*s}" stroke-width="{1.8*s:.1f}"/>')
    add(f'<path d="M{ax},{ay} L{bx0+30*s},{beam-10*s} M{ax},{ay} L{bx0+150*s},{beam-10*s}" stroke-width="{1.1*s:.1f}" opacity="0.8"/>')
    # the trolley, its cables, and the box on the hook
    tx = bx0 + 150 * s
    hook_y = beam + 120 * s
    add(f'<rect x="{tx-14*s}" y="{beam}" width="{28*s}" height="{12*s}" stroke-width="{1.4*s:.1f}"/>')
    add(f'<path d="M{tx-8*s},{beam+12*s} V{hook_y} M{tx+8*s},{beam+12*s} V{hook_y}" stroke-width="{1*s:.1f}" opacity="0.8"/>')
    add(f'<rect x="{tx-27*s}" y="{hook_y}" width="{54*s}" height="{27*s}" rx="2" fill="{hexa(PINK,0.35)}" stroke="{PINK}" stroke-width="{1.6*s:.1f}"/>')
    add('</g>')
    # warning lights
    add(f'<g filter="url(#glow)"><circle cx="{ax}" cy="{ay-6*s}" r="{3.2*s:.1f}" fill="{WARM}"/><circle cx="{bx0+6*s}" cy="{beam-14*s}" r="{2.6*s:.1f}" fill="{PINK}"/></g>')

crane(QX + 130, 1.0, CYAN)
crane(QX + 330, 0.8, BLUE)

# ---------------------------------------------------------------- reflections
add('<g clip-path="url(#below-horizon)">')
for cx, y, c in lit:
    reflect(cx + CW/2, KEEL + 4, c, 160 + random.uniform(0, 80), 4, 0.55)
for i, cx in enumerate(cols):
    if i % 2 == 0:
        reflect(cx + CW/2, KEEL + 2, random.choice([BLUE, PURPLE]), 90, 2, 0.3)
for bx in (QX + 130, QX + 330):
    reflect(bx - 320 + 6, DECK + 10, PINK, 120, 2, 0.35)
reflect(SX + 80, KEEL + 2, PINK, 120, 2, 0.4)
reflect(BOW + 18, KEEL + 2, CYAN, 100, 2, 0.4)
# and the ship's hull, softly, as a dark mirrored mass
add(f'<polygon points="{poly([(x, 2*KEEL - y + 10) for x, y in hull])}" fill="{hexa(CYAN,0.05)}" stroke="{hexa(CYAN,0.18)}" stroke-width="1.2" filter="url(#blur-soft)"/>')
add('</g>')

# ---------------------------------------------------------------- finish
add(f'<rect width="{W}" height="{H}" fill="url(#vignette)"/>')
add('</svg>')

open(OUT, 'w').write('\n'.join(out))
print('elements:', sum(s.count('<') for s in out), 'bytes:', sum(len(s) for s in out))
