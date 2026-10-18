<#
.SYNOPSIS
    qa-seed-admin.ps1 — автосид qa-админа на стенде edu-platform (T45).

.DESCRIPTION
    Готовит учётную запись администратора, под которой qa-abc.ps1 прогоняет
    админ-блоки, без ручных шагов (в P4 учётку заводили руками: регистрация +
    PATCH роли). Алгоритм:

      1. POST /auth/login под AdminEmail -> 200: учётка готова, выход 0
         (повторный запуск идемпотентен и ничего не меняет).
      2. Иначе вход сид-админом (-SeedAdminEmail/-SeedAdminPassword — те же
         значения, что ADMIN_EMAIL/ADMIN_PASSWORD при запуске стенда).
         Неудача -> понятная ошибка и выход 1.
      3. GET /admin/users?q=<AdminEmail>: поиск учётки в списке пользователей.
         Если не найдена — POST /auth/register (обычная регистрация,
         role=student).
      4. PATCH /admin/users/{id}/role {"role":"admin"} — повышение до админа
         (идемпотентно: для действующего админа тоже 200).
      5. Контрольный POST /auth/login под AdminEmail -> 200, role=admin:
         выход 0, иначе 1.

    Все не-GET запросы шлют Origin: http://localhost:3000 (CSRF-защита стенда;
    origin входит в ALLOWED_ORIGINS по умолчанию). На 429 (лимит /auth/* —
    10/мин на IP) пауза ~20 с и повтор, до 4 попыток.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail
    Email qa-админа — учётка, которую готовим. Передавайте тот же, что
    -AdminEmail в qa-abc.ps1. По умолчанию qa-admin@example.test.

.PARAMETER AdminPassword
    Пароль qa-админа. По умолчанию QaAdmin12345 (не короче 8 символов —
    требование POST /auth/register).

.PARAMETER SeedAdminEmail
    Email сид-админа стенда (ADMIN_EMAIL окружения api). По умолчанию admin@local.

.PARAMETER SeedAdminPassword
    Пароль сид-админа стенда (ADMIN_PASSWORD окружения api). По умолчанию admin1234.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-seed-admin.ps1

.EXAMPLE
    # Свежая база / другая учётка: те же сид-креды, другой qa-админ.
    .\qa-seed-admin.ps1 -AdminEmail "qa-seed-1@example.test"

.EXAMPLE
    # Сид-админ стенда задан не дефолтными кредами:
    .\qa-seed-admin.ps1 -SeedAdminEmail "root@local" -SeedAdminPassword "s3cret-pass"

.NOTES
    Совместим с PowerShell 5.1, внешних зависимостей нет. Коды возврата:
    0 — учётка существует, роль admin, вход по паролю работает;
    1 — не удалось (текст ошибки в stdout).
#>
param(
    [string]$BaseUrl = "http://localhost:8082",
    [string]$AdminEmail = "qa-admin@example.test",
    [string]$AdminPassword = "QaAdmin12345",
    [string]$SeedAdminEmail = "admin@local",
    [string]$SeedAdminPassword = "admin1234"
)

$ErrorActionPreference = "Stop"

# ---- HTTP-обёртка -----------------------------------------------------------
# Возвращает объект {Status, Body, Json, Headers}. Ошибки HTTP не исключение,
# а статус + тело: проверяются и 4xx/5xx. (Тот же контракт, что в qa-abc/qa-p4.)
function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [object]$Body = $null,
        $Session = $null,
        [hashtable]$Headers = @{}
    )
    $uri = $script:BaseUrl + $Path

    # CSRF: у не-GET выставляем Origin из ALLOWED_ORIGINS стенда.
    $allHeaders = @{}
    foreach ($k in $Headers.Keys) { $allHeaders[$k] = $Headers[$k] }
    if (-not $allHeaders.ContainsKey("Origin")) { $allHeaders["Origin"] = "http://localhost:3000" }

    $params = @{
        Method          = $Method
        Uri             = $uri
        Headers         = $allHeaders
        UseBasicParsing = $true
    }
    if ($Session -ne $null) { $params.WebSession = $Session }
    if ($Body -ne $null) {
        # PS 5.1 кодирует тело в ISO-8859-1 — отдаём явные UTF-8 байты,
        # иначе кириллица превращается в «?».
        $params.Body = [System.Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 8))
        $params.ContentType = "application/json; charset=utf-8"
    }

    $status = 0; $text = ""; $respHeaders = $null
    try {
        $resp = Invoke-WebRequest @params
        $status = [int]$resp.StatusCode
        $text = $resp.Content
        $respHeaders = $resp.Headers
    } catch {
        $ex = $_.Exception.Response
        if ($ex -ne $null) {
            $status = [int]$ex.StatusCode
            try {
                $stream = $ex.GetResponseStream()
                if ($stream -ne $null) {
                    $stream.Position = 0
                    $reader = New-Object System.IO.StreamReader($stream)
                    $text = $reader.ReadToEnd()
                }
            } catch { }
            if ((-not $text) -and $_.ErrorDetails -ne $null -and $_.ErrorDetails.Message) {
                $text = $_.ErrorDetails.Message
            }
        } else {
            $text = $_.Exception.Message
        }
    }

    $json = $null
    if ($text) {
        try { $json = $text | ConvertFrom-Json } catch { $json = $null }
    }
    return [pscustomobject]@{
        Status  = $status
        Body    = $text
        Json    = $json
        Headers = $respHeaders
    }
}

# ---- /auth/* с повтором на rate-limit ---------------------------------------
# Стенд ограничивает вход/регистрацию (RATE_LIMIT_AUTH_PER_MIN = 10/мин на IP).
# При 429 ждём окно и повторяем — сид не должен зависеть от соседних прогонов.
function Invoke-Auth {
    param([string]$Method, [string]$Path, [object]$Body = $null, $Session = $null)
    $r = $null
    for ($i = 1; $i -le 4; $i++) {
        $r = Invoke-Api -Method $Method -Path $Path -Body $Body -Session $Session
        if ($r.Status -ne 429) { return $r }
        if ($i -lt 4) {
            Write-Host "  (лимит аутентификации: пауза 20 с, попытка $i из 4)" -ForegroundColor DarkYellow
            Start-Sleep -Seconds 20
        }
    }
    return $r
}

function Fail-Seed {
    param([string]$Message)
    Write-Host "[ОШИБКА] $Message" -ForegroundColor Red
    Write-Host "Автосид qa-админа не выполнен (exit 1)." -ForegroundColor Red
    exit 1
}

# ---- Шаги -------------------------------------------------------------------
Write-Host "=== qa-seed-admin: подготовка qa-админа ===" -ForegroundColor Cyan
Write-Host "API: $BaseUrl | qa-админ: $AdminEmail | сид-админ: $SeedAdminEmail"
Write-Host ""

# 0. Стенд должен отвечать.
$ping = Invoke-Api -Method GET -Path "/healthz"
if ($ping.Status -ne 200) {
    Fail-Seed "Стенд недоступен: $BaseUrl/healthz -> $($ping.Status). Запустите docker compose up -d db api runner."
}

# 1. Быстрый путь: учётка уже готова.
$qaSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Session $qaSession `
    -Body @{ email = $AdminEmail; password = $AdminPassword }
if ($r.Status -eq 200) {
    Write-Host "[1/5] Вход под $AdminEmail -> 200: учётка готова, изменений не требуется." -ForegroundColor Green
    exit 0
}
if ($r.Status -eq 429) {
    Fail-Seed "Лимит /auth/* не отпустил после 4 попыток (429) — повторите запуск позже."
}
Write-Host "[1/5] Вход под $AdminEmail -> $($r.Status): готовим учётку." -ForegroundColor Yellow

# 2. Вход сид-админом.
$seedSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Session $seedSession `
    -Body @{ email = $SeedAdminEmail; password = $SeedAdminPassword }
if ($r.Status -eq 429) {
    Fail-Seed "Лимит /auth/* не отпустил после 4 попыток (429) — повторите запуск позже."
}
if ($r.Status -ne 200) {
    Fail-Seed "Вход сид-админом ($SeedAdminEmail) -> $($r.Status): $($r.Body). Проверьте -SeedAdminEmail/-SeedAdminPassword: они должны совпадать с ADMIN_EMAIL/ADMIN_PASSWORD стенда."
}
$seedRole = ""
if ($r.Json -ne $null) { $seedRole = [string]$r.Json.role }
if ($seedRole -ne "admin") {
    Fail-Seed "Учётка сид-админа $SeedAdminEmail имеет роль '$seedRole', а не admin — нужен администратор стенда."
}
Write-Host "[2/5] Вход сид-админом -> 200." -ForegroundColor Green

# 3. Поиск целевой учётки; если нет — регистрация.
$q = [uri]::EscapeDataString($AdminEmail)
$targetID = ""
$r = Invoke-Api -Method GET -Path "/api/v1/admin/users?q=$q" -Session $seedSession
if ($r.Status -ne 200) {
    Fail-Seed "GET /admin/users?q=$AdminEmail -> $($r.Status): $($r.Body)"
}
if ($r.Json -ne $null -and $r.Json.items -ne $null) {
    foreach ($item in @($r.Json.items)) {
        if ([string]$item.email -ieq $AdminEmail) { $targetID = [string]$item.id }
    }
}

if ($targetID -ne "") {
    Write-Host "[3/5] Учётка $AdminEmail найдена (id=$targetID) — регистрация не нужна." -ForegroundColor Green
} else {
    $regSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Session $regSession `
        -Body @{ email = $AdminEmail; password = $AdminPassword }
    if ($r.Status -eq 201 -and $r.Json -ne $null) {
        $targetID = [string]$r.Json.id
    }
    if ($targetID -eq "") {
        # Повторный поиск: страховка от гонки «появился между поиском и
        # регистрацией» (тогда register вернул 400 «уже существует»).
        $r2 = Invoke-Api -Method GET -Path "/api/v1/admin/users?q=$q" -Session $seedSession
        if ($r2.Status -eq 200 -and $r2.Json -ne $null -and $r2.Json.items -ne $null) {
            foreach ($item in @($r2.Json.items)) {
                if ([string]$item.email -ieq $AdminEmail) { $targetID = [string]$item.id }
            }
        }
        if ($targetID -eq "") {
            Fail-Seed "Регистрация $AdminEmail -> $($r.Status): $($r.Body)"
        }
    }
    Write-Host "[3/5] Учётка $AdminEmail зарегистрирована (id=$targetID)." -ForegroundColor Green
}

# 4. Роль admin.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$targetID/role" -Session $seedSession `
    -Body @{ role = "admin" }
if ($r.Status -ne 200) {
    Fail-Seed "PATCH /admin/users/$targetID/role -> $($r.Status): $($r.Body)"
}
Write-Host "[4/5] Роль admin назначена." -ForegroundColor Green

# 5. Контрольный вход под целевой учёткой.
$qaSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Session $qaSession `
    -Body @{ email = $AdminEmail; password = $AdminPassword }
if ($r.Status -eq 200 -and $r.Json -ne $null -and [string]$r.Json.role -eq "admin") {
    Write-Host "[5/5] Контрольный вход $AdminEmail -> 200, role=admin." -ForegroundColor Green
    Write-Host ""
    Write-Host "=== Готово: $AdminEmail — администратор стенда (exit 0) ===" -ForegroundColor Cyan
    exit 0
}
Fail-Seed "Контрольный вход $AdminEmail -> $($r.Status): $($r.Body)"
