<#
.SYNOPSIS
    qa-p4.ps1 - сквозной QA-прогон раздела P4 (задачи T31-T40).

.DESCRIPTION
    Дополняет qa-abc.ps1 (базовый сценарий) и qa-p3.ps1 (P3) проверкой того,
    что появилось в волне P4, на живом стенде через API:

      H. Журнал платежей (T31): поиск по студенту (q) без учёта регистра,
         период from/to с включительными границами, totals по отфильтрованным
         данным, 400 на мусорную дату и from > to, регрессии status/пагинации.
      I. Карточка курса (T33): при archived PATCH -> 409 «карточка заморожена»,
         чтение курса не ломается, после возврата из архива правка снова
         работает.
      J. Супер-админка (T34/T32): счётчик users.blocked в GET /admin/stats
         считается (дельта при блокировке/разблокировке), вход заблокированного
         пользователя -> 403 «Учётная запись заблокирована».

    Фронтендовая часть волны (T36-T40) этим скриптом не проверяется: она
    закрыта `npm test` (контрактные тесты API-клиента, T37) и ручной приёмкой
    UI в браузере.

    Сценарий создаёт собственные сущности с уникальной меткой времени
    (p4-teacher / p4-student / p4-dummy), поэтому его можно гонять повторно
    на том же стенде.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER Origin
    Origin для CSRF-заголовков у не-GET запросов (ALLOWED_ORIGINS стенда).

.PARAMETER SkipWeb
    Не проверять доступность фронтенда (порт 3001).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-p4.ps1
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
# а статус + тело: проверяем и 4xx/5xx. (Тот же контракт, что в qa-abc/qa-p3.)
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
# скрипт запускают сразу после qa-abc/qa-p3, приходит 429 «слишком много
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

# =============================================================================
$stamp = (Get-Date).ToString("yyyyMMddHHmmss") + "-" + (Get-Random -Maximum 9999)
$teacherEmail = "p4-teacher-$stamp@example.test"
$studentEmail = "p4-student-$stamp@example.test"
$dummyEmail   = "p4-dummy-$stamp@example.test"
$studentName  = "P4name-$stamp"
$password     = "password123"
$today     = (Get-Date).ToString("yyyy-MM-dd")
$tomorrow  = (Get-Date).AddDays(1).ToString("yyyy-MM-dd")
$yesterday = (Get-Date).AddDays(-1).ToString("yyyy-MM-dd")
$qStudent  = [uri]::EscapeDataString($studentEmail)
$qUpper    = [uri]::EscapeDataString($studentEmail.ToUpperInvariant())
$qName     = [uri]::EscapeDataString($studentName.ToLowerInvariant())

Write-Host "=== qa-p4: сквозной QA раздела P4 (T31-T40) ===" -ForegroundColor Cyan
Write-Host "API: $BaseUrl | Origin: $Origin | Метка: $stamp"
Write-Host "Преподаватель: $teacherEmail"
Write-Host "Ученик: $studentEmail"
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
Write-Host "--- Подготовка: админ, преподаватель, ученик ---" -ForegroundColor Cyan

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

$studentSession = New-Session
$studentId = ""
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Session $studentSession `
    -Body @{ email = $studentEmail; password = $password }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $studentId = [string]$r.Json.id }
$regOk = ($r.Status -eq 201 -and $studentId -ne "")
# displayName задаётся отдельно (T23): контракт регистрации лишние поля
# отклоняет, поэтому имя — через PATCH /me/profile.
$profileOk = $false
if ($regOk) {
    $r = Invoke-Api -Method PATCH -Path "/api/v1/me/profile" -Session $studentSession `
        -Body @{ displayName = $studentName }
    $profileOk = ($r.Status -eq 200 -and $r.Json.displayName -eq $studentName)
}
Invoke-Check "P4. Ученик: регистрация 201 + displayName через /me/profile" `
    ($regOk -and $profileOk) $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- H. Журнал платежей: поиск и период (T31) ---" -ForegroundColor Cyan

# H1. Бесплатный курс преподавателя (нужен платёж в журнале).
$courseID = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = "P4 курс $stamp"; description = "Сквозной курс волны P4."; priceCents = 0 }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $courseID = [string]$r.Json.id }
Invoke-Check "H1. Создание бесплатного курса -> 201" `
    ($r.Status -eq 201 -and $courseID -ne "") $r.Body

# H2. Программа курса: submit требует минимум один модуль и один урок.
$moduleID = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/modules" -Session $teacherSession `
    -Body @{ title = "Модуль P4" }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $moduleID = [string]$r.Json.id }
$lessonID = ""
if ($moduleID -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons" -Session $teacherSession `
        -Body @{ moduleId = $moduleID; title = "Урок P4"; kind = "video_text"; bodyMd = "## Урок P4" }
    if ($r.Status -eq 201 -and $r.Json -ne $null) { $lessonID = [string]$r.Json.id }
}
Invoke-Check "H2. Модуль и урок курса -> 201" `
    ($moduleID -ne "" -and $lessonID -ne "") $r.Body

# H3. Подача на модерацию: draft -> review.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/submit" -Session $teacherSession -Body @{}
Invoke-Check "H3. Подача курса на модерацию -> 200, review" `
    ($r.Status -eq 200 -and $r.Json.status -eq "review") $r.Body

# H4. Approve админом: review -> published.
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/moderate" -Session $adminSession `
    -Body @{ decision = "approve" }
Invoke-Check "H4. Approve админом -> 200, published" `
    ($r.Status -eq 200 -and $r.Json.status -eq "published") $r.Body

# H5. Запись ученика на бесплатный курс: платёж создаётся сразу со succeeded.
$freeProviderID = "free-$studentId-$courseID"
$r = Invoke-Api -Method POST -Path "/api/v1/enrollments" -Session $studentSession `
    -Body @{ courseId = $courseID }
$enrollOk = ($r.Status -eq 201 -and $r.Json -ne $null -and
             [string]$r.Json.providerPaymentId -eq $freeProviderID -and
             [string]$r.Json.status -eq "succeeded" -and $r.Json.enrollmentActive -eq $true)
Invoke-Check "H5. Запись на бесплатный курс -> 201, succeeded, доступ открыт" $enrollOk $r.Body

# H6. Поиск по email ученика: ровно наша запись с ключевыми полями.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qStudent" -Session $adminSession
$item = $null
if ($r.Status -eq 200 -and @($r.Json.items).Count -eq 1) { $item = @($r.Json.items)[0] }
$itemOk = ($item -ne $null -and [string]$item.studentEmail -eq $studentEmail -and
           [string]$item.courseId -eq $courseID -and [string]$item.status -eq "succeeded" -and
           [string]$item.providerPaymentId -eq $freeProviderID -and
           ($item.PSObject.Properties.Name -contains "courseTitle") -and
           ($item.PSObject.Properties.Name -contains "studentName"))
Invoke-Check "H6. q=email -> 1 платёж с полями studentEmail/courseId/status" `
    ($r.Status -eq 200 -and [int]$r.Json.total -eq 1 -and $itemOk) $r.Body

# H7. Поиск без учёта регистра (email заглавными).
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qUpper" -Session $adminSession
Invoke-Check "H7. q=EMAIL заглавными -> найден (без учёта регистра)" `
    ($r.Status -eq 200 -and [int]$r.Json.total -eq 1) $r.Body

# H8. Поиск по displayName ученика (строчными буквами).
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qName" -Session $adminSession
Invoke-Check "H8. q=displayName строчными -> найден" `
    ($r.Status -eq 200 -and [int]$r.Json.total -eq 1) $r.Body

# H9. Несуществующий студент -> пусто.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=zzz-nope-$stamp" -Session $adminSession
Invoke-Check "H9. q=несуществующий -> total=0" `
    ($r.Status -eq 200 -and [int]$r.Json.total -eq 0) $r.Body

# H10-H12. Период: включительные границы, будущее и прошлое отсекаются.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qStudent&from=$today&to=$today" -Session $adminSession
Invoke-Check "H10. from=сегодня&to=сегодня -> платёж входит" `
    ($r.Status -eq 200 -and [int]$r.Json.total -eq 1) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qStudent&from=$tomorrow" -Session $adminSession
Invoke-Check "H11. from=завтра -> платёж отсекается" `
    ($r.Status -eq 200 -and [int]$r.Json.total -eq 0) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qStudent&to=$yesterday" -Session $adminSession
Invoke-Check "H12. to=вчера -> платёж отсекается" `
    ($r.Status -eq 200 -and [int]$r.Json.total -eq 0) $r.Body

# H13-H14. Ошибки разбора.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?from=2026-13-45&q=$qStudent" -Session $adminSession
Invoke-Check "H13. Мусорная дата -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?from=$tomorrow&to=$today&q=$qStudent" -Session $adminSession
Invoke-Check "H14. from > to -> 400" ($r.Status -eq 400) $r.Body

# H15. totals считаются по отфильтрованным данным.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qStudent" -Session $adminSession
$totalsOk = $false
if ($r.Status -eq 200 -and $r.Json.totals -ne $null) {
    $names = $r.Json.totals.PSObject.Properties.Name
    $sum = 0
    foreach ($it in @($r.Json.items)) { $sum += [int]$it.amountCents }
    $totalsOk = (($names -contains "amountCents") -and ($names -contains "platformFeeCents") -and
                 ($names -contains "teacherAmountCents") -and ([int]$r.Json.totals.amountCents -eq $sum))
}
Invoke-Check "H15. totals по фильтру: amountCents == сумме items" `
    ($r.Status -eq 200 -and $totalsOk) $r.Body

# H16. Регрессия: status-фильтр рядом с q.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qStudent&status=succeeded" -Session $adminSession
$ok1 = ($r.Status -eq 200 -and [int]$r.Json.total -eq 1)
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?q=$qStudent&status=failed" -Session $adminSession
$ok2 = ($r.Status -eq 200 -and [int]$r.Json.total -eq 0)
Invoke-Check "H16. q + status=succeeded/failed -> 1/0" ($ok1 -and $ok2) $r.Body

# H17. Регрессия: пагинация (все страницы не пересекаются).
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?limit=1&page=1" -Session $adminSession
if ($r.Status -eq 200 -and [int]$r.Json.total -ge 2 -and @($r.Json.items).Count -eq 1) {
    $id1 = [string]@($r.Json.items)[0].id
    $r2 = Invoke-Api -Method GET -Path "/api/v1/admin/payments?limit=1&page=2" -Session $adminSession
    $id2 = ""
    if ($r2.Status -eq 200 -and @($r2.Json.items).Count -eq 1) { $id2 = [string]@($r2.Json.items)[0].id }
    Invoke-Check "H17. Пагинация limit=1: стр.1 и стр.2 различаются" `
        ($id1 -ne "" -and $id2 -ne "" -and $id1 -ne $id2) $r.Body
} else {
    Skip-Check "H17. Пагинация limit=1: стр.1 и стр.2 различаются" "в журнале меньше двух платежей"
}

# H18. Регрессия доступа: преподавателю журнал недоступен.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments" -Session $teacherSession
Invoke-Check "H18. Преподавателю GET /admin/payments -> 403" ($r.Status -eq 403) $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- I. Карточка курса: архив замораживает правки (T33) ---" -ForegroundColor Cyan

# I1. Правка опубликованного курса разрешена.
$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseID" -Session $teacherSession `
    -Body @{ title = "P4 курс v2 $stamp" }
Invoke-Check "I1. PATCH опубликованного курса -> 200" ($r.Status -eq 200) $r.Body

# I2. В архив.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/archive" -Session $teacherSession
Invoke-Check "I2. Archive -> 200, status=archived" `
    ($r.Status -eq 200 -and $r.Json.course.status -eq "archived") $r.Body

# I3-I4. В архиве карточка заморожена: любая правка -> 409.
$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseID" -Session $teacherSession `
    -Body @{ title = "P4 запрещённая правка $stamp" }
$frozenMsg = ($r.Body -match "заморожен")
Invoke-Check "I3. PATCH заголовка в архиве -> 409 «заморожена»" `
    ($r.Status -eq 409 -and $frozenMsg) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseID" -Session $teacherSession `
    -Body @{ priceCents = 12345 }
Invoke-Check "I4. PATCH цены в архиве -> 409" ($r.Status -eq 409) $r.Body

# I5. Чтение курса из архива не ломается.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID" -Session $teacherSession
Invoke-Check "I5. GET курса в архиве -> 200, status=archived" `
    ($r.Status -eq 200 -and $r.Json.course.status -eq "archived" -and [string]$r.Json.course.title -eq "P4 курс v2 $stamp") $r.Body

# I6-I7. Возврат из архива снова открывает правки.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/unarchive" -Session $teacherSession
$unarchiveOk = ($r.Status -eq 200 -and $r.Json.course.status -eq "draft")
Invoke-Check "I6. Unarchive -> 200, status=draft (возврат всегда в черновик)" $unarchiveOk $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseID" -Session $teacherSession `
    -Body @{ title = "P4 курс v3 $stamp" }
Invoke-Check "I7. PATCH после возврата из архива -> 200" ($r.Status -eq 200) $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- J. Счётчик заблокированных и вход блокировки (T34/T32) ---" -ForegroundColor Cyan

# J1. Базовое значение users.blocked.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/stats" -Session $adminSession
$blockedKeyExists = $false
$blockedBase = -1
if ($r.Status -eq 200 -and $r.Json.users -ne $null -and
    ($r.Json.users.PSObject.Properties.Name -contains "blocked")) {
    $blockedKeyExists = $true
    $blockedBase = [int]$r.Json.users.blocked
}
Invoke-Check "J1. GET /admin/stats: users.blocked присутствует" `
    ($r.Status -eq 200 -and $blockedKeyExists) $r.Body

# J2. Подопытный пользователь.
$dummySession = New-Session
$dummyId = ""
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Session $dummySession `
    -Body @{ email = $dummyEmail; password = $password }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $dummyId = [string]$r.Json.id }
Invoke-Check "J2. Регистрация подопытного -> 201" `
    ($r.Status -eq 201 -and $dummyId -ne "") $r.Body

# J3. Блокировка админом.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$dummyId/status" -Session $adminSession `
    -Body @{ status = "blocked" }
Invoke-Check "J3. Блокировка -> 200, status=blocked" `
    ($r.Status -eq 200 -and $r.Json.user.status -eq "blocked") $r.Body

# J4. Счётчик вырос ровно на один.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/stats" -Session $adminSession
$blockedNow = -1
if ($r.Status -eq 200 -and $r.Json.users -ne $null) { $blockedNow = [int]$r.Json.users.blocked }
Invoke-Check "J4. После блокировки users.blocked = база + 1" `
    ($blockedBase -ge 0 -and $blockedNow -eq ($blockedBase + 1)) "было $blockedBase, стало $blockedNow | $($r.Body)"

# J5. Вход заблокированного -> 403 с единым текстом (T32).
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" `
    -Body @{ email = $dummyEmail; password = $password }
Invoke-Check "J5. Вход заблокированного -> 403 «Учётная запись заблокирована»" `
    ($r.Status -eq 403 -and $r.Body -match "заблокирована") $r.Body

# J6-J8. Разблокировка возвращает счётчик и доступ.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$dummyId/status" -Session $adminSession `
    -Body @{ status = "active" }
$unblockOk = ($r.Status -eq 200 -and $r.Json.user.status -eq "active")
$r = Invoke-Api -Method GET -Path "/api/v1/admin/stats" -Session $adminSession
$blockedAfter = -1
if ($r.Status -eq 200 -and $r.Json.users -ne $null) { $blockedAfter = [int]$r.Json.users.blocked }
Invoke-Check "J6. Разблокировка -> 200, users.blocked вернулся к базе" `
    ($unblockOk -and $blockedAfter -eq $blockedBase) "было $blockedBase, стало $blockedAfter | $($r.Body)"

$loginSession = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Session $loginSession `
    -Body @{ email = $dummyEmail; password = $password }
Invoke-Check "J7. Вход после разблокировки -> 200" ($r.Status -eq 200) $r.Body

# =============================================================================
Write-Host ""
$total = $script:Passed + $script:Failed + $script:Skipped
Write-Host "=== qa-p4: $script:Passed успешно, $script:Failed провалено, $script:Skipped пропущено (из $total) ===" `
    -ForegroundColor Cyan

if ($script:Failed -gt 0) {
    Write-Host "Есть падения" -ForegroundColor Red
    exit 1
}
Write-Host "Все проверки пройдены" -ForegroundColor Green
exit 0
