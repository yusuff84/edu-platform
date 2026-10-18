<#
.SYNOPSIS
    qa-p6.ps1 - сквозной QA-прогон волны P6 (задачи T47-T52).

.DESCRIPTION
    Дополняет qa-abc.ps1 (базовый сценарий), qa-p3.ps1, qa-p4.ps1 и qa-p5.ps1
    проверкой того, что появилось в волне P6, на живом стенде через API:

      M. Приглашения (T47): создание приглашения -> оно видно в
         GET /admin/invites (без токена, с tokenPrefix), отзыв -> revoked,
         повторный отзыв -> 409, чужой id -> 404, не-админ -> 403,
         учитель регистрируется именно по этому приглашению.
      N. Курсы админки (T48): сортировка sort/dir (title asc/desc на двух
         своих курсах), 400 на кривой sort/dir, фильтр teacherEmail,
         период from/to (включительно) и 400 на кривую дату/перевёрнутый
         период, очередь модерации с teacherEmail/modulesCount/lessonsCount,
         archive -> unarchive (archived -> draft), повторный unarchive -> 409,
         не-админ на /admin/courses -> 403.
      O. Записи на курсы (T48): approve курса, покупка mock, вебхук,
         q по ученику и status=active в GET /admin/enrollments,
         400 на кривой статус, не-админ -> 403.
      P. Возврат платежа (T47): refund -> payment.refunded + запись refunded
         (курс исчез из /me/courses), повтор -> 409, чужой id -> 404,
         не-админ -> 403.
      Q. Веб-стенд: недоступность считается SKIP (как в остальных qa-скриптах).

    Сценарий создаёт собственные сущности с уникальной меткой времени
    (qa-p6-префикс), поэтому его можно гонять повторно на том же стенде.
    В конце черновой курс удаляется, опубликованный — архивируется
    (best-effort, в счёт проверок не входит).

    Важно: возврат средств возможен только для платежа, созданного после
    последнего перезапуска api (реестр mock-провайдера живёт в памяти
    процесса) — скрипт покупает курс сам, поэтому ограничение не мешает.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER Origin
    Origin для CSRF-заголовков у не-GET запросов (ALLOWED_ORIGINS стенда).

.PARAMETER SkipWeb
    Не проверять доступность фронтенда (порт 3001).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-p6.ps1
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
# а статус + тело: проверяем и 4xx/5xx. (Тот же контракт, что в qa-abc/qa-p3/qa-p4/qa-p5.)
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
# скрипт запускают сразу после других qa-скриптов, приходит 429 «слишком много
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
$stamp = Get-Date -Format "yyyyMMddHHmmss"
$teacherEmail = "qa-p6-teacher-$stamp@example.test"
$studentEmail = "qa-p6-student-$stamp@example.test"
$password = "password123"
$titleA = "qa-p6-Aaa-$stamp"
$titleB = "qa-p6-Bbb-$stamp"
$today = (Get-Date).ToString("yyyy-MM-dd")
$tomorrow = (Get-Date).AddDays(1).ToString("yyyy-MM-dd")

$adminSession = New-Session
$teacherSession = New-Session
$studentSession = New-Session

$courseA = ""
$courseB = ""
$paymentId = ""
$approveOk = $false

Write-Host "=== qa-p6: волна P6 ($stamp) ===" -ForegroundColor Cyan

# =============================================================================
Write-Host "`n--- M. Приглашения (T47) ---" -ForegroundColor Cyan

# M1. Вход администратора.
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Session $adminSession `
    -Body @{ email = $AdminEmail; password = $AdminPassword }
$adminOk = ($r.Status -eq 200)
Invoke-Check "M1. Вход администратора ($AdminEmail) -> 200" $adminOk $r.Body
if (-not $adminOk) {
    Write-Host "Без администратора прогон невозможен" -ForegroundColor Red
    exit 1
}

# M2. Создание приглашения учителя.
$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Session $adminSession `
    -Body @{ email = $teacherEmail }
$inviteToken = ""
if ($r.Status -eq 201 -and $r.Json -ne $null) { $inviteToken = [string]$r.Json.token }
Invoke-Check "M2. POST /admin/invites -> 201 с токеном" `
    ($r.Status -eq 201 -and $inviteToken -ne "") $r.Body

# M3. Регистрация учителя по этому приглашению.
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Session $teacherSession `
    -Body @{ email = $teacherEmail; password = $password; inviteToken = $inviteToken }
Invoke-Check "M3. Регистрация учителя по приглашению -> 201" `
    ($r.Status -eq 201 -and $r.Json.role -eq "teacher") $r.Body

# M4. Список приглашений: запись есть, без токена, с tokenPrefix.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/invites?limit=100" -Session $adminSession
$invite = $null
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $invite = @($r.Json.items) | Where-Object { $_.email -eq $teacherEmail } | Select-Object -First 1
}
$inviteOk = $false
if ($invite -ne $null) {
    $hasToken = ($invite.PSObject.Properties["token"] -ne $null) -or ($invite.PSObject.Properties["tokenHash"] -ne $null)
    $inviteOk = ($invite.status -eq "sent") -and ($invite.tokenPrefix -ne $null) -and `
                ($invite.expiresAt -ne $null) -and ($invite.createdAt -ne $null) -and (-not $hasToken)
}
Invoke-Check "M4. GET /admin/invites: запись есть, статус sent, без токена" $inviteOk $r.Body

# M5. Отзыв приглашения.
$inviteId = ""
if ($invite -ne $null) { $inviteId = [string]$invite.id }
if ($inviteId -eq "") {
    Skip-Check "M5. POST /admin/invites/{id}/revoke -> 200 revoked" "приглашение не найдено (M2/M4)"
} else {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/invites/$inviteId/revoke" -Session $adminSession
    Invoke-Check "M5. POST /admin/invites/{id}/revoke -> 200 revoked" `
        ($r.Status -eq 200 -and $r.Json.invite.status -eq "revoked") $r.Body

    # M6. Повторный отзыв -> 409.
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/invites/$inviteId/revoke" -Session $adminSession
    Invoke-Check "M6. Повторный отзыв -> 409" ($r.Status -eq 409) $r.Body

    # M7. Несуществующее приглашение -> 404.
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/invites/$([guid]::NewGuid())/revoke" -Session $adminSession
    Invoke-Check "M7. Отзыв несуществующего приглашения -> 404" ($r.Status -eq 404) $r.Body
}

# M8. Учитель на /admin/invites -> 403.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/invites" -Session $teacherSession
Invoke-Check "M8. Учитель на GET /admin/invites -> 403" ($r.Status -eq 403) $r.Body

# =============================================================================
Write-Host "`n--- N. Курсы админки: сортировка, фильтры, unarchive, очередь (T48) ---" -ForegroundColor Cyan

# N1. Черезвый учитель создаёт два курса (A — на модерацию, B — черновик).
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = $titleA; description = "QA P6: модерация и покупка"; priceCents = 10000 }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $courseA = [string]$r.Json.id; $slugA = [string]$r.Json.slug }
Invoke-Check "N1. POST /teacher/courses (A) -> 201" ($r.Status -eq 201 -and $courseA -ne "") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = $titleB; description = "QA P6: сортировка"; priceCents = 20000 }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $courseB = [string]$r.Json.id }
Invoke-Check "N2. POST /teacher/courses (B) -> 201" ($r.Status -eq 201 -and $courseB -ne "") $r.Body

# N3. Модуль и урок в курсе A (для modulesCount/lessonsCount в очереди).
$moduleId = ""
if ($courseA -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/modules" -Session $teacherSession `
        -Body @{ title = "Модуль QA" }
    if ($r.Status -eq 201 -and $r.Json -ne $null) { $moduleId = [string]$r.Json.id }
}
Invoke-Check "N3. POST /teacher/courses/{A}/modules -> 201" `
    ($r.Status -eq 201 -and $moduleId -ne "") $r.Body

if ($moduleId -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Session $teacherSession `
        -Body @{ moduleId = $moduleId; title = "Урок QA"; kind = "video_text"; bodyMd = "Текст урока QA P6." }
}
Invoke-Check "N4. POST /teacher/courses/{A}/lessons -> 201" ($r.Status -eq 201) $r.Body

# N5. Отправка курса A на модерацию.
if ($courseA -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/submit" -Session $teacherSession
}
Invoke-Check "N5. Submit курса A -> review" `
    ($r.Status -eq 200 -and $r.Json.status -eq "review") $r.Body

# N6. Сортировка title asc по своим курсам (фильтр teacherEmail).
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?teacherEmail=$teacherEmail&sort=title&dir=asc&limit=100" -Session $adminSession
$items = @()
if ($r.Status -eq 200 -and $r.Json -ne $null) { $items = @($r.Json.items) }
$ascOk = ($r.Status -eq 200 -and $items.Count -ge 2 -and [string]$items[0].title -eq $titleA)
Invoke-Check "N6. Сортировка title asc: первым идёт A" $ascOk $r.Body

# N7. Сортировка title desc.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?teacherEmail=$teacherEmail&sort=title&dir=desc&limit=100" -Session $adminSession
$descOk = $false
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $items = @($r.Json.items)
    $descOk = ($items.Count -ge 2 -and [string]$items[0].title -eq $titleB)
}
Invoke-Check "N7. Сортировка title desc: первым идёт B" $descOk $r.Body

# N8. Невалидный sort -> 400.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?sort=stars" -Session $adminSession
Invoke-Check "N8. sort=stars -> 400" ($r.Status -eq 400) $r.Body

# N9. Невалидный dir -> 400.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?dir=sideways" -Session $adminSession
Invoke-Check "N9. dir=sideways -> 400" ($r.Status -eq 400) $r.Body

# N10. Фильтр teacherEmail: только свои курсы.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?teacherEmail=$teacherEmail&limit=100" -Session $adminSession
$filterOk = $false
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $items = @($r.Json.items)
    $filterOk = ($items.Count -ge 2) -and (@($items | Where-Object { $_.teacherEmail -ne $teacherEmail }).Count -eq 0)
}
Invoke-Check "N10. teacherEmail: все записи — свои" $filterOk $r.Body

# N11. Период from/to включает сегодняшние курсы.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?teacherEmail=$teacherEmail&from=$today&to=$today&limit=100" -Session $adminSession
Invoke-Check "N11. from/to = сегодня -> курсы найдены" `
    ($r.Status -eq 200 -and $r.Json.total -ge 2) $r.Body

# N12. from=завтра -> пусто.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?teacherEmail=$teacherEmail&from=$tomorrow" -Session $adminSession
Invoke-Check "N12. from=завтра -> пусто" ($r.Status -eq 200 -and $r.Json.total -eq 0) $r.Body

# N13. from позже to -> 400.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?from=$tomorrow&to=$today" -Session $adminSession
Invoke-Check "N13. from > to -> 400" ($r.Status -eq 400) $r.Body

# N14. Очередь модерации: письмо автора и объём курса.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/review" -Session $adminSession
$queueItem = $null
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $queueItem = @($r.Json.items) | Where-Object { $_.id -eq $courseA } | Select-Object -First 1
}
$queueOk = $false
if ($queueItem -ne $null) {
    $queueOk = ($queueItem.teacherEmail -eq $teacherEmail) -and `
               ([int]$queueItem.modulesCount -ge 1) -and ([int]$queueItem.lessonsCount -ge 1)
}
Invoke-Check "N14. Очередь: teacherEmail + modulesCount/lessonsCount" $queueOk $r.Body

# N15. unarchive: B -> в архив -> из архива.
$unarchiveOk = $false
if ($courseB -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseB/archive" -Session $adminSession
    if ($r.Status -eq 200 -and $r.Json.course.status -eq "archived") {
        $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseB/unarchive" -Session $adminSession
        $unarchiveOk = ($r.Status -eq 200 -and $r.Json.course.status -eq "draft")
    }
}
Invoke-Check "N15. archive -> unarchive: назад в draft" $unarchiveOk $r.Body

# N16. Повторный unarchive (курс уже draft) -> 409.
if ($courseB -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseB/unarchive" -Session $adminSession
}
Invoke-Check "N16. Повторный unarchive -> 409" ($r.Status -eq 409) $r.Body

# N17. Учитель на GET /admin/courses -> 403.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses" -Session $teacherSession
Invoke-Check "N17. Учитель на GET /admin/courses -> 403" ($r.Status -eq 403) $r.Body

# =============================================================================
Write-Host "`n--- O. Записи на курсы (T48) ---" -ForegroundColor Cyan

# O1. Одобрение курса A.
if ($courseA -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/moderate" -Session $adminSession `
        -Body @{ decision = "approve" }
    $approveOk = ($r.Status -eq 200 -and $r.Json.status -eq "published")
}
Invoke-Check "O1. Approve курса A -> published" $approveOk $r.Body

# O2. Регистрация ученика.
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Session $studentSession `
    -Body @{ email = $studentEmail; password = $password }
Invoke-Check "O2. Регистрация ученика -> 201" ($r.Status -eq 201) $r.Body

# O3. Покупка курса A (mock).
$providerPaymentId = ""
if ($approveOk -and $courseA -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/enrollments" -Session $studentSession `
        -Body @{ courseId = $courseA; provider = "mock" }
    if ($r.Status -eq 201 -and $r.Json -ne $null) { $providerPaymentId = [string]$r.Json.providerPaymentId }
}
Invoke-Check "O3. Покупка mock -> 201" ($r.Status -eq 201 -and $providerPaymentId -ne "") $r.Body

# O4. Вебхук «оплата прошла».
if ($providerPaymentId -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" `
        -Body @{ providerPaymentId = $providerPaymentId; status = "succeeded" }
}
Invoke-Check "O4. Вебхук succeeded -> processed" `
    ($r.Status -eq 200 -and $r.Json.status -eq "processed") $r.Body

# O5. Список записей: поиск по ученику.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/enrollments?q=$studentEmail&limit=100" -Session $adminSession
$enrollmentItem = $null
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $enrollmentItem = @($r.Json.items) | Where-Object { $_.studentEmail -eq $studentEmail } | Select-Object -First 1
}
$enrollmentOk = ($enrollmentItem -ne $null) -and ($enrollmentItem.courseId -eq $courseA) -and `
                ($enrollmentItem.status -eq "active") -and ($enrollmentItem.percent -eq 0)
Invoke-Check "O5. q=ученик: запись active на курс A" $enrollmentOk $r.Body

# O6. Фильтр status=active.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/enrollments?q=$studentEmail&status=active&limit=100" -Session $adminSession
Invoke-Check "O6. status=active -> запись найдена" `
    ($r.Status -eq 200 -and $r.Json.total -ge 1) $r.Body

# O7. Невалидный статус -> 400.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/enrollments?status=expired" -Session $adminSession
Invoke-Check "O7. status=expired -> 400" ($r.Status -eq 400) $r.Body

# O8. Учитель на /admin/enrollments -> 403.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/enrollments" -Session $teacherSession
Invoke-Check "O8. Учитель на GET /admin/enrollments -> 403" ($r.Status -eq 403) $r.Body

# =============================================================================
Write-Host "`n--- P. Возврат платежа (T47) ---" -ForegroundColor Cyan

# P1. Находим успешный платёж по курсу A в журнале.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?courseId=$courseA&limit=100" -Session $adminSession
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $succeeded = @($r.Json.items) | Where-Object { $_.status -eq "succeeded" } | Select-Object -First 1
    if ($succeeded -ne $null) { $paymentId = [string]$succeeded.id }
}
Invoke-Check "P1. В журнале есть успешный платёж курса A" ($paymentId -ne "") $r.Body

# P2. Возврат средств.
$refundOk = $false
if ($paymentId -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/payments/$paymentId/refund" -Session $adminSession
    $refundOk = ($r.Status -eq 200 -and $r.Json.payment.status -eq "refunded" -and $r.Json.payment.processedAt -ne $null)
}
Invoke-Check "P2. Refund -> 200, статус refunded, processedAt заполнен" $refundOk $r.Body

# P3. Повторный возврат -> 409.
if ($paymentId -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/payments/$paymentId/refund" -Session $adminSession
}
Invoke-Check "P3. Повторный refund -> 409" ($r.Status -eq 409) $r.Body

# P4. Несуществующий платёж -> 404.
$r = Invoke-Api -Method POST -Path "/api/v1/admin/payments/$([guid]::NewGuid())/refund" -Session $adminSession
Invoke-Check "P4. Refund несуществующего платежа -> 404" ($r.Status -eq 404) $r.Body

# P5. Курс исчез из «моих курсов» ученика.
$r = Invoke-Api -Method GET -Path "/api/v1/me/courses" -Session $studentSession
$goneOk = $false
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $mine = @($r.Json.items) | Where-Object { $_.course.id -eq $courseA }
    $goneOk = ($mine.Count -eq 0)
}
Invoke-Check "P5. После возврата курс недоступен ученику" $goneOk $r.Body

# P6. Запись перешла в status=refunded.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/enrollments?q=$studentEmail&status=refunded&limit=100" -Session $adminSession
Invoke-Check "P6. status=refunded: запись найдена" ($r.Status -eq 200 -and $r.Json.total -ge 1) $r.Body

# P7. Учитель на refund -> 403.
if ($paymentId -ne "") {
    $r = Invoke-Api -Method POST -Path "/api/v1/admin/payments/$paymentId/refund" -Session $teacherSession
}
Invoke-Check "P7. Учитель на refund -> 403" ($r.Status -eq 403) $r.Body

# ---- Приборка (best-effort; в счёт проверок не входит) ----------------------
if ($courseB -ne "") {
    $null = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseB" -Session $teacherSession
}
if ($approveOk -and $courseA -ne "") {
    $null = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/archive" -Session $adminSession
}

# =============================================================================
Write-Host "`n--- Q. Веб-стенд ---" -ForegroundColor Cyan

if ($SkipWeb) {
    Skip-Check "Q1. Фронтенд на localhost:3001 отвечает" "выключено параметром -SkipWeb"
} else {
    $web = Invoke-Api -Method GET -Path "" -AbsoluteUri "http://localhost:3001/"
    if ($web.Status -eq 0) {
        Skip-Check "Q1. Фронтенд на localhost:3001 отвечает" "веб-сервер недоступен (не запущен)"
    } else {
        Invoke-Check "Q1. Фронтенд на localhost:3001 отвечает" ($web.Status -eq 200) "status=$($web.Status)"
    }
}

# =============================================================================
Write-Host ""
$total = $script:Passed + $script:Failed + $script:Skipped
Write-Host "=== qa-p6: $script:Passed успешно, $script:Failed провалено, $script:Skipped пропущено (из $total) ===" `
    -ForegroundColor Cyan

if ($script:Failed -gt 0) {
    Write-Host "Есть падения" -ForegroundColor Red
    exit 1
}
Write-Host "Все проверки пройдены" -ForegroundColor Green
exit 0
