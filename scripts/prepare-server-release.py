"""Prepare a private database/uploads/source/image bundle for the IP server."""
import gzip
import argparse
import hashlib
import json
import secrets
import shutil
import subprocess
import tarfile
from pathlib import Path

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output-root', type=Path, default=root / '.local')
args = parser.parse_args()
release = '20261118-' + secrets.token_hex(4)
stage = args.output_root / ('server-' + release)
stage.mkdir(parents=True)


def command(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def save(args, filename):
    with (stage / filename).open('wb') as output:
        command(args, stdout=output)


volume = subprocess.check_output(['docker', 'inspect', 'edu-review-api-1', '--format', '{{range .Mounts}}{{if eq .Destination "/uploads"}}{{.Name}}{{end}}{{end}}'], text=True).strip()
if not volume:
    raise RuntimeError('Missing source uploads volume')
command(['docker', 'pause', 'edu-review-api-1'], stdout=subprocess.DEVNULL)
try:
    save(['docker', 'exec', 'edu-review-db-1', 'pg_dump', '-U', 'edu', '-d', 'edu', '-Fc', '--no-owner', '--no-acl'], 'database.dump')
    save(['docker', 'run', '--rm', '--network', 'none', '-v', volume + ':/source:ro', '--entrypoint', 'tar', 'edu-review-web', '-czf', '-', '-C', '/source', '.'], 'uploads.tar.gz')
finally:
    command(['docker', 'unpause', 'edu-review-api-1'], stdout=subprocess.DEVNULL)

images = []
for service in ['api', 'web', 'runner']:
    image = 'skillex-' + service + ':' + release
    command(['docker', 'tag', 'edu-review-' + service, image])
    images.append(image)
with gzip.open(stage / 'images.tar.gz', 'wb', compresslevel=3) as output:
    process = subprocess.Popen(['docker', 'save', *images], stdout=subprocess.PIPE)
    shutil.copyfileobj(process.stdout, output)
    if process.wait():
        raise RuntimeError('docker save failed')

files = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard'], cwd=root, text=True).splitlines()
with tarfile.open(stage / 'source.tar.gz', 'w:gz') as archive:
    for name in files:
        file = root / name
        if not file.is_file() or file.suffix in {'.exe', '.pyc', '.log', '.dump'}:
            continue
        if file.name.startswith('.env') and file.name not in {'.env.example', '.env.test'}:
            continue
        archive.add(file, arcname=name, recursive=False)

admin_password = secrets.token_urlsafe(24)
env = {'RELEASE': release, 'POSTGRES_PASSWORD': secrets.token_hex(24), 'SESSION_SECRET': secrets.token_hex(48),
       'ADMIN_EMAIL': 'admin@skillex.space', 'ADMIN_PASSWORD': admin_password}
(stage / '.env').write_text(''.join(key + '=' + value + '\n' for key, value in env.items()), encoding='utf-8')
(stage / 'admin-access.txt').write_text('URL: http://132.243.195.86/login\nEmail: ' + env['ADMIN_EMAIL'] + '\nPassword: ' + admin_password + '\n', encoding='utf-8')
manifest = {'release': release, 'sourceDatabase': 'edu-review-db-1', 'sourceUploadsVolume': volume, 'files': {}}
for file in stage.iterdir():
    if file.name not in {'manifest.json'}:
        digest = hashlib.sha256()
        with file.open('rb') as stream:
            for block in iter(lambda: stream.read(1024 * 1024), b''):
                digest.update(block)
        manifest['files'][file.name] = {'bytes': file.stat().st_size, 'sha256': digest.hexdigest()}
(stage / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
bundle = stage.with_suffix('.tar')
with tarfile.open(bundle, 'w') as archive:
    archive.add(stage, arcname=release)
print(json.dumps({'release': release, 'directory': str(stage), 'bundle': str(bundle), 'bytes': bundle.stat().st_size}))
