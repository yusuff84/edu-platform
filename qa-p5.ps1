<#
.SYNOPSIS
    qa-p5.ps1 - сквозной QA-прогон раздела P5 (задачи T41-T46).

.DESCRIPTION
    Дополняет qa-abc.ps1 (базовый сценарий), qa-p3.ps1 (P3) и qa-p4.ps1 (P4)
    проверкой того, что появилось в волне P5, на живом стенде через API:

      K. Slug (T41): кириллический slug транслитерируется («Мой-Курс-» ->
         «moy-kurs-»), авто-slug строится из названия курса, повторная отправка
         того же slug -> 400, slug без латиницы -> 400, PATCH меняет slug с
         транслитерацией, публичная страница /courses/{slug} открывается по
         новому человекочитаемому слагу.
      L. CORS-префлайт (T42): Access-Control-Allow-Methods перечисляет реальные
         методы маршрута (GET для каталога, POST для логина, GET+POST для
         /teacher/courses, без DELETE на логине), неизвестный путь -> без
         заголовка, чужой Origin -> без Allow-Origin.

    Остальные задачи волны этим скриптом не проверяются: T43 — ручная приёмка
    UI + tsc/vite, T44 — docker-прогон тестов runner, T45 — автосид qa-админа
    (qa-abc.ps1), T46 — интеграционные тесты Postgres (TEST_DATABASE_URL).

    Сценарий создаёт собственные сущности с уникальной меткой времени
    (p5-префикс), поэтому его можно гонять повторно на том же стенде.
    В конце черновой курс удаляется, опубликованный — архивируется
    (best-effort, в счёт проверок не входит).

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER Origin
    Origin для CSRF-заголовков у не-GET запросов (ALLOWED_ORIGINS стенда).

.PARAMETER SkipWeb
    Не проверять доступность фронтенда (порт 3001).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-p5.ps1
#>
param(
    [string]$BaseUrl = "http://localhost:8082",
    [string]$AdminEmail = "admin@local",
    [string]$AdminPassword = "admin1234",
    [string]$Origin = "http://localhost:3000",
    [switch]$SkipWeb
)

$ErrorActionPreference = "Stop"

# ---- Счётчики ---------------------------------------------------------------
$script:Passed = 0
$script:Failed = 0
$script:Skipped = 0

function Invoke-Check {
    param([string]$Name, [bool]$Ok, [string]$Detail = "")
    if ($Ok) {
        $script:Passed++
        Write-Host "[PASS] $Name" -ForegroundColor Green
    } else {
        $script:Failed++
        if ($Detail) {
            $short = $Detail
            if ($short.Length -gt 240) { $short = $short.Substring(0, 240) + "..." }
            Write-Host "[FAIL] $Name - $short" -ForegroundColor Red
        } else {
            Write-Host "[FAIL] $Name" -ForegroundColor Red
        }
    }
}

function Skip-Check {
    param([string]$Name, [string]$Reason)
    $script:Skipped++
    Write-Host "[SKIP] $Name - $Reason" -ForegroundColor Yellow
}

# ---- HTTP-обёртка -----------------------------------------------------------
# Возвращает объект {Status, Body, Json, Headers}. Ошибки HTTP не исключение,
# а статус + тело: проверяем и 4xx/5xx. (Тот же контракт, что в qa-abc/qa-p3/qa-p4.)
function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [object]$Body = $null,
        $Session = $null,
        [hashtable]$Headers = @{},
        [string]$AbsoluteUri = ""
    )
    $uri = $AbsoluteUri
    if (-not $uri) { $uri = $script:BaseUrl + $Path }

    # CSRF: у не-GET выставляем Origin из ALLOWED_ORIGINS стенда.
    $allHeaders = @{}
    foreach ($k in $Headers.Keys) { $allHeaders[$k] = $Headers[$k] }
    if (-not $allHeaders.ContainsKey("Origin")) { $allHeaders["Origin"] = $Origin }

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

    $status = 0; $text = ""
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
        $respHeaders = $null
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

# ---- Новые сессии -----------------------------------------------------------
function New-Session { return New-Object Microsoft.PowerShell.Commands.WebRequestSession }

# ---- Обёртка для /auth/* с повтором на rate-limit ---------------------------
# Стенд ограничивает аутентификацию (RATE_LIMIT_AUTH_PER_MIN, по IP). Если
# скрипт запускают сразу после qa-abc/qa-p3/qa-p4, приходит 429 «слишком много
# запросов» — ждём окно и повторяем, чтобы приёмка не зависела от порядка
# запуска скриптов.
function Invoke-Auth {
    param([string]$Method, [string]$Path, [object]$Body = $null, $Session = $null)
    $r = $null
    for ($i = 1; $i -le 4; $i++) {
        $r = Invoke-Api -Method $Method -Path $Path -Body $Body -Session $Session
        if ($r.Status -ne 429) { return $r }
        Write-Host "  (лимит аутентификации: пауза 20 с, попытка $i)" -ForegroundColor DarkYellow
        Start-Sleep -Seconds 20
    }
    return $r
}

# ---- Чтение заголовка ответа (значение может быть массивом строк) -----------
function Get-RespHeader {
    param($Resp, [string]$Name)
    if ($Resp -eq $null -or $Resp.Headers -eq $null) { return "" }
    $v = $Resp.Headers[$Name]
    if ($v -eq $null) { return "" }
    return [string]::Join(", ", @($v)).Trim()
}

# =============================================================================
$stamp = (Get-Date).ToString("yyyyMMddHHmmss") + "-" + (Get-Random -Maximum 9999)
$teacherEmail = "p5-teacher-$stamp@example.test"
$password     = "password123"
$slugCyrillic = "Мой-Курс-$stamp"
$slugExpected = "moy-kurs-$stamp"
$slugPatched  = "Обновлённый Адрес $stamp"
$slugPatchedExpected = "obnovlennyy-adres-$stamp"
$autoTitle    = "Тест Авто Slug $stamp"
$autoExpected = "test-avto-slug-$stamp"

Write-Host "=== qa-p5: сквозной QA раздела P5 (T41-T46) ===" -ForegroundColor Cyan
Write-Host "API: $BaseUrl | Origin: $Origin | Метка: $stamp"
Write-Host "Преподаватель: $teacherEmail"
Write-Host ""

$ping = Invoke-Api -Method GET -Path "/healthz"
if ($ping.Status -ne 200) {
    Write-Host "Стенд недоступен: $BaseUrl/healthz -> $($ping.Status)" -ForegroundColor Red
    exit 1
}

if (-not $SkipWeb) {
    try {
        $web = Invoke-WebRequest -Uri "http://localhost:3001" -UseBasicParsing -TimeoutSec 5
        Invoke-Check "P0. Фронтенд отвечает" ([int]$web.StatusCode -eq 200)
    } catch {
        Skip-Check "P0. Фронтенд отвечает" "web не поднят/другой порт — для API-прогона допустимо"
    }
}

# =============================================================================
Write-Host ""
Write-Host "--- Подготовка: админ, преподаватель ---" -ForegroundColor Cyan

$adminSession = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Session $adminSession `
    -Body @{ email = $AdminEmail; password = $AdminPassword }
Invoke-Check "P1. Вход админа -> 200" ($r.Status -eq 200) $r.Body

# Приглашение: роль teacher выдаётся только по invite-токену.
$inviteToken = ""
$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Session $adminSession `
    -Body @{ email = $teacherEmail }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $inviteToken = [string]$r.Json.token }
Invoke-Check "P2. Приглашение преподавателя -> 201 с токеном" `
    ($r.Status -eq 201 -and $inviteToken -ne "") $r.Body

$teacherSession = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Session $teacherSession `
    -Body @{ email = $teacherEmail; password = $password; inviteToken = $inviteToken }
Invoke-Check "P3. Регистрация преподавателя по приглашению -> 201" `
    ($r.Status -eq 201) $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- K. Slug: транслитерация кириллицы (T41) ---" -ForegroundColor Cyan

# K1. Кириллический slug транслитерируется.
$courseA = ""
$slugA = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = "Курс P5 кириллица $stamp"; description = "Сквозной курс волны P5."; priceCents = 0; slug = $slugCyrillic }
if ($r.Status -eq 201 -and $r.Json -ne $null) {
    $courseA = [string]$r.Json.id
    $slugA = [string]$r.Json.slug
}
Invoke-Check "K1. Cyrillic slug «$slugCyrillic» -> 201, slug=$slugExpected" `
    ($r.Status -eq 201 -and $slugA -eq $slugExpected) "получен slug='$slugA' | $($r.Body)"

# K2. Пустой slug строится из названия курса.
$courseB = ""
$slugB = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = $autoTitle; description = "Авто-slug из названия."; priceCents = 0 }
if ($r.Status -eq 201 -and $r.Json -ne $null) {
    $courseB = [string]$r.Json.id
    $slugB = [string]$r.Json.slug
}
Invoke-Check "K2. Авто-slug из названия -> 201, slug=$autoExpected" `
    ($r.Status -eq 201 -and $slugB -eq $autoExpected) "получен slug='$slugB' | $($r.Body)"

# K3. Повторная отправка того же slug -> 400.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = "Дубль $stamp"; description = "Дубль slug."; priceCents = 0; slug = $slugCyrillic }
Invoke-Check "K3. Занятый slug -> 400" ($r.Status -eq 400) $r.Body

# K4. Slug без единого латинского символа -> 400.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = "Мусор $stamp"; description = "Мусорный slug."; priceCents = 0; slug = "+++" }
Invoke-Check "K4. Slug «+++» (после транслитерации пусто) -> 400" ($r.Status -eq 400) $r.Body

# K5. PATCH меняет slug с транслитерацией.
$patchedOk = $false
if ($courseA -ne "") {
    $r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseA" -Session $teacherSession `
        -Body @{ slug = $slugPatched }
    $patchStatus = $r.Status
    $r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseA" -Session $teacherSession
    $nowSlug = ""
    if ($r.Status -eq 200 -and $r.Json.course -ne $null) { $nowSlug = [string]$r.Json.course.slug }
    $patchedOk = ($patchStatus -eq 200 -and $nowSlug -eq $slugPatchedExpected)
}
Invoke-Check "K5. PATCH slug «$slugPatched» -> slug=$slugPatchedExpected" $patchedOk $r.Body

# K6. Программа курса A: модуль + урок (нужны для публикации).
$moduleID = ""
$lessonID = ""
if ($courseA -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/modules" -Session $teacherSession `
        -Body @{ title = "Модуль P5" }
    if ($r.Status -eq 201 -and $r.Json -ne $null) { $moduleID = [string]$r.Json.id }
    if ($moduleID -ne "") {
        $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Session $teacherSession `
            -Body @{ moduleId = $moduleID; title = "Урок P5"; kind = "video_text"; bodyMd = "## Урок P5" }
        if ($r.Status -eq 201 -and $r.Json -ne $null) { $lessonID = [string]$r.Json.id }
    }
}
Invoke-Check "K6. Модуль и урок курса A -> 201" ($moduleID -ne "" -and $lessonID -ne "") $r.Body

# K7. Подача на модерацию и публикация (нужны для публичной страницы).
$submitOk = $false
if ($courseA -ne "" -and $moduleID -ne "" -and $lessonID -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/submit" -Session $teacherSession -Body @{}
    $submitOk = ($r.Status -eq 200 -and $r.Json.status -eq "review")
}
Invoke-Check "K7. Подача курса A -> 200, review" $submitOk $r.Body

$approveOk = $false
if ($submitOk) {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/moderate" -Session $adminSession `
        -Body @{ decision = "approve" }
    $approveOk = ($r.Status -eq 200 -and $r.Json.status -eq "published")
}
Invoke-Check "K8. Approve админом -> 200, published" $approveOk $r.Body

# K9. Публичная страница открывается по человекочитаемому слагу.
$publicOk = $false
if ($approveOk) {
    $r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPatchedExpected"
    $pubSlug = ""
    if ($r.Status -eq 200 -and $r.Json -ne $null) {
        if ($r.Json.slug) { $pubSlug = [string]$r.Json.slug }
        elseif ($r.Json.course -ne $null) { $pubSlug = [string]$r.Json.course.slug }
    }
    $publicOk = ($r.Status -eq 200 -and $pubSlug -eq $slugPatchedExpected)
}
Invoke-Check "K9. GET /courses/$slugPatchedExpected -> 200" $publicOk $r.Body

# ---- Приборка (best-effort; в счёт проверок не входит) ----------------------
if ($courseB -ne "") {
    $null = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseB" -Session $teacherSession
}
if ($approveOk -and $courseA -ne "") {
    $null = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/archive" -Session $teacherSession
}

# =============================================================================
Write-Host ""
Write-Host "--- L. CORS-префлайт: маршрутные Allow-Methods (T42) ---" -ForegroundColor Cyan

$acrm = "POST"
$r = Invoke-Api -Method OPTIONS -Path "/api/v1/courses" -Headers @{ "Access-Control-Request-Method" = $acrm }
$am = Get-RespHeader $r "Access-Control-Allow-Methods"
$ao = Get-RespHeader $r "Access-Control-Allow-Origin"
$ac = Get-RespHeader $r "Access-Control-Allow-Credentials"
Invoke-Check "L1. OPTIONS /courses -> 204, methods=GET (без POST), origin+credentials" `
    ($r.Status -eq 204 -and $am -match "GET" -and -not ($am -match "POST") -and `
     $ao -eq $Origin -and $ac -eq "true") "status=$($r.Status) methods='$am' origin='$ao' creds='$ac'"

$acrm = "POST"
$r = Invoke-Api -Method OPTIONS -Path "/api/v1/auth/login" -Headers @{ "Access-Control-Request-Method" = $acrm }
$am = Get-RespHeader $r "Access-Control-Allow-Methods"
Invoke-Check "L2. OPTIONS /auth/login -> 204, methods=POST (без GET)" `
    ($r.Status -eq 204 -and $am -match "POST" -and -not ($am -match "GET")) `
    "status=$($r.Status) methods='$am'"

$acrm = "POST"
$r = Invoke-Api -Method OPTIONS -Path "/api/v1/teacher/courses" -Headers @{ "Access-Control-Request-Method" = $acrm }
$am = Get-RespHeader $r "Access-Control-Allow-Methods"
Invoke-Check "L3. OPTIONS /teacher/courses -> 204, methods содержат GET и POST" `
    ($r.Status -eq 204 -and $am -match "GET" -and $am -match "POST") `
    "status=$($r.Status) methods='$am'"

$acrm = "DELETE"
$r = Invoke-Api -Method OPTIONS -Path "/api/v1/auth/login" -Headers @{ "Access-Control-Request-Method" = $acrm }
$am = Get-RespHeader $r "Access-Control-Allow-Methods"
Invoke-Check "L4. OPTIONS /auth/login: метода DELETE в списке нет" `
    ($am -match "POST" -and -not ($am -match "DELETE")) "methods='$am'"

$acrm = "GET"
$r = Invoke-Api -Method OPTIONS -Path "/api/v1/no-such-path-$stamp" -Headers @{ "Access-Control-Request-Method" = $acrm }
$am = Get-RespHeader $r "Access-Control-Allow-Methods"
Invoke-Check "L5. OPTIONS неизвестного пути -> без Allow-Methods" `
    ($am -eq "") "status=$($r.Status) methods='$am'"

$acrm = "GET"
$evilOrigin = "http://evil-$stamp.example"
$r = Invoke-Api -Method OPTIONS -Path "/api/v1/courses" `
    -Headers @{ "Access-Control-Request-Method" = $acrm; "Origin" = $evilOrigin }
$ao = Get-RespHeader $r "Access-Control-Allow-Origin"
Invoke-Check "L6. OPTIONS чужого Origin -> без Allow-Origin" ($ao -eq "") "origin='$ao'"

# =============================================================================
Write-Host ""
$total = $script:Passed + $script:Failed + $script:Skipped
Write-Host "=== qa-p5: $script:Passed успешно, $script:Failed провалено, $script:Skipped пропущено (из $total) ===" `
    -ForegroundColor Cyan

if ($script:Failed -gt 0) {
    Write-Host "Есть падения" -ForegroundColor Red
    exit 1
}
Write-Host "Все проверки пройдены" -ForegroundColor Green
exit 0
