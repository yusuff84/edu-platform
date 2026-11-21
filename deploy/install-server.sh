#!/usr/bin/env bash
set -euo pipefail
umask 077
release=${1:?release required}
[[ "$release" =~ ^20261118-[a-f0-9]{8}$ ]] || exit 2
base=/opt/skillex
transfer=$base/transfers/$release
target=$base/releases/$release
backup=$base/backups/hamlion-before-$release
bundle=/root/skillex-transfer-$release.tar
install -d -m 700 "$base/transfers" "$base/releases" "$base/backups"
[[ ! -e "$base/current" ]] || { echo 'Existing skillex deployment: refusing to overwrite'; exit 3; }
tar -xf "$bundle" -C "$base/transfers"
python3 - "$transfer" <<'PY'
import hashlib,json,sys
from pathlib import Path
root=Path(sys.argv[1])
manifest=json.loads((root/'manifest.json').read_text())
for name,entry in manifest['files'].items():
    file=(root/name).resolve()
    assert file.parent==root.resolve()
    assert file.stat().st_size==entry['bytes'],name
    h=hashlib.sha256()
    with file.open('rb') as f:
        for block in iter(lambda:f.read(1024*1024),b''):h.update(block)
    assert h.hexdigest()==entry['sha256'],name
print('TRANSFER_CHECKSUMS_OK')
PY
install -d -m 700 "$target" "$backup"
tar -xzf "$transfer/source.tar.gz" -C "$target"
patch=/root/skillex-patch-$release.tar.gz
echo 'WAITING_FOR_VERIFIED_SERVER_CONFIGURATION'
for i in {1..180}; do
    [[ -f "$patch.ready" ]] && break
    sleep 5
done
[[ -f "$patch.ready" ]] || exit 6
sha256sum -c "$patch.sha256"
tar -xzf "$patch" -C "$target"
chmod 0755 "$target/artifacts/api" "$target/artifacts/runner"
install -m 600 "$transfer/.env" "$base/.env"
tar -czf "$backup/nginx.tar.gz" -C /etc nginx
docker inspect hamlion-web hamlion-worker hamlion-postgres hamlion-redis > "$backup/containers.json"
tar -czf "$backup/application-config.tar.gz" -C /opt/hamlion .env docker-compose.yml docker-compose.prod.yml
echo 'OLD_CONFIGURATION_SAVED'
docker stop hamlion-web hamlion-worker
docker exec hamlion-postgres sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -Fc' > "$backup/database.dump"
docker exec hamlion-postgres sh -c 'exec pg_dumpall -U "$POSTGRES_USER" --globals-only' > "$backup/database-globals.sql"
tar -czf "$backup/uploads.tar.gz" -C /var/lib/docker/volumes/hamlion_uploads/_data .
docker stop hamlion-postgres hamlion-redis
tar -czf "$backup/redis.tar.gz" -C /var/lib/docker/volumes/hamlion_redisdata/_data .
sha256sum "$backup"/*.gz "$backup"/*.dump "$backup"/*.sql > "$backup/SHA256SUMS"
echo 'OLD_APPLICATION_BACKED_UP_AND_STOPPED'
if [[ -f "$transfer/images.tar.gz" ]]; then
    docker load -i "$transfer/images.tar.gz"
else
    for service in api web runner; do
        docker build -t "skillex-$service:$release" -f "$target/deploy/Dockerfile.$service-prebuilt" "$target"
    done
fi
docker build --network none -t "skillex-api:$release" -f "$target/deploy/Dockerfile.api-prebuilt" "$target"
compose=(docker compose -p skillex --env-file "$base/.env" -f "$target/deploy/compose.server.yml")
"${compose[@]}" up -d db
for i in {1..60}; do
    if "${compose[@]}" exec -T db pg_isready -U skillex -d skillex >/dev/null 2>&1; then break; fi
    sleep 2
done
tables=$("${compose[@]}" exec -T db psql -U skillex -d skillex -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';")
[[ "$tables" == '0' ]] || { echo 'New database is not empty; refusing restore'; exit 4; }
"${compose[@]}" exec -T db pg_restore -U skillex -d skillex --no-owner --no-acl --exit-on-error --single-transaction < "$transfer/database.dump"
"${compose[@]}" exec -T db psql -U skillex -d skillex -v ON_ERROR_STOP=1 -c "UPDATE users SET status='blocked' WHERE split_part(lower(email),'@',2) IN ('local','e2e.test','example.test');"
echo 'FULL_DATABASE_RESTORED_TEST_LOGINS_BLOCKED'
"${compose[@]}" create api
docker run --rm --network none -i -v skillex_uploads:/destination --entrypoint tar "skillex-web:$release" -xzf - -C /destination < "$transfer/uploads.tar.gz"
docker run --rm --network none -v skillex_uploads:/destination --entrypoint chown "skillex-web:$release" -R 1000:1000 /destination
echo 'UPLOADS_RESTORED'
install -m 644 "$target/deploy/apparmor-skillex-runner" /etc/apparmor.d/skillex-runner
apparmor_parser -r /etc/apparmor.d/skillex-runner
"${compose[@]}" up -d --wait --wait-timeout 180
ln -s "$target" "$base/current"
if [[ -L /etc/nginx/sites-enabled/hamlion.com ]]; then
    [[ "$(readlink /etc/nginx/sites-enabled/hamlion.com)" == '/etc/nginx/sites-available/hamlion.com' ]] || exit 5
    unlink /etc/nginx/sites-enabled/hamlion.com
fi
install -m 644 "$target/deploy/nginx-http.conf" /etc/nginx/sites-available/skillex-ip
ln -s /etc/nginx/sites-available/skillex-ip /etc/nginx/sites-enabled/skillex-ip
nginx -t
systemctl reload nginx
curl --fail --silent -H 'Host: 132.243.195.86' http://127.0.0.1/api/v1/courses/cifrovaya-gigiena > "$transfer/course-check.json"
"${compose[@]}" ps
printf '%s\n' 'DEPLOYMENT_READY http://132.243.195.86'
