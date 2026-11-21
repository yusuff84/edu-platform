"""Package a saved DB snapshot, recovered uploads and prebuilt Linux binaries."""
import hashlib
import json
import secrets
import shutil
import subprocess
import tarfile
from pathlib import Path

root = Path(__file__).resolve().parents[1]
release = '20261118-5b628dcc'
stage = Path('D:/CodexDeploy/skillex/server-' + release)
for binary in ['api', 'runner']:
    with (stage / 'artifacts' / binary).open('rb') as stream:
        assert stream.read(4) == b'\x7fELF', binary
shutil.copyfile(root / '.local/server-20261118-09a67d46/database.dump', stage / 'database.dump')
with (stage / 'database.dump').open('rb') as stream:
    assert stream.read(5) == b'PGDMP'
# Verify that every downloaded course media file survived recovery intact.
course = root / '.local/stepik-146168'
assets = json.loads((course / 'assets.json').read_text(encoding='utf-8'))
expected = {'imports/stepik-146168/' + Path(a['file']).name: a for a in assets}
checked = set()
upload_count = 0
with tarfile.open(stage / 'uploads.tar.gz', 'r|gz') as archive:
    for member in archive:
        if not member.isfile():
            continue
        upload_count += 1
        name = member.name.removeprefix('./')
        if name not in expected:
            continue
        asset = expected[name]
        assert member.size == asset['bytes'], name
        saved = hashlib.sha256()
        stream = archive.extractfile(member)
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            saved.update(block)
        original = hashlib.sha256((course / asset['file']).read_bytes()).digest()
        assert saved.digest() == original, name
        checked.add(name)
assert checked == set(expected)
print('RECOVERED_UPLOADS_VERIFIED', upload_count, flush=True)
files = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard'], cwd=root, text=True).splitlines()
with tarfile.open(stage / 'source.tar.gz', 'w:gz') as archive:
    for name in files:
        file = root / name
        if not file.is_file() or file.suffix in {'.exe', '.pyc', '.log', '.dump'}:
            continue
        if file.name.startswith('.env') and file.name not in {'.env.example', '.env.test'}:
            continue
        archive.add(file, arcname=name, recursive=False)
    archive.add(root / 'frontend/dist', arcname='frontend/dist')
    archive.add(stage / 'artifacts', arcname='artifacts')
if not (stage / '.env').exists():
    admin_password = secrets.token_urlsafe(24)
    env = {'RELEASE': release, 'POSTGRES_PASSWORD': secrets.token_hex(24), 'SESSION_SECRET': secrets.token_hex(48),
           'ADMIN_EMAIL': 'admin@skillex.space', 'ADMIN_PASSWORD': admin_password}
    (stage / '.env').write_text(''.join(key + '=' + value + '\n' for key, value in env.items()), encoding='utf-8')
    (stage / 'admin-access.txt').write_text('URL: http://132.243.195.86/login\nEmail: ' + env['ADMIN_EMAIL'] + '\nPassword: ' + admin_password + '\n', encoding='utf-8')
manifest = {'release': release, 'sourceDatabase': 'edu-review-db-1', 'uploadCount': upload_count, 'files': {}}
names = ['.env', 'admin-access.txt', 'database.dump', 'uploads.tar.gz', 'source.tar.gz']
if (stage / 'images.tar.gz').is_file():
    names.append('images.tar.gz')
for name in names:
    file = stage / name
    digest = hashlib.sha256()
    with file.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(block)
    manifest['files'][name] = {'bytes': file.stat().st_size, 'sha256': digest.hexdigest()}
(stage / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
bundle = stage.with_suffix('.tar')
with tarfile.open(bundle, 'w') as archive:
    for name in [*manifest['files'], 'manifest.json']:
        archive.add(stage / name, arcname=release + '/' + name)
print(json.dumps({'release': release, 'bundle': str(bundle), 'bytes': bundle.stat().st_size}))
