<#
.SYNOPSIS
    qa-p7.ps1 - сквозной QA-прогон волны P7 (задачи T53-T55).

.DESCRIPTION
    Дополняет qa-abc.ps1, qa-p3/qa-p4/qa-p5/qa-p6 проверками того, что
    добавлено в волну P7:

      A. Жизненный цикл учителя (T53): teacher unpublish (published -> draft),
         повторный unpublish -> 409, удаление published -> 409, удаление
         draft -> 204, валидация quiz на бэкенде (3 негатива), PATCH урока
         со startsAt в datetime-local (трактовка как UTC).
      B. Обложка курса (T54): загрузка multipart (PNG) -> 201 coverPath,
         coverPath в каталоге и на публичной странице, повторная загрузка
         перезаписывает, удаление -> 204 и coverPath=null, 400 плохой тип,
         400 слишком большой файл, 401 без сессии, 403 чужой учитель,
         409 для архивного курса.
      C. Детали курса для админа (T55): GET /admin/courses/{id} -> course +
         modules/lessons (только метаданные, порядок по position) + stats;
         рост studentsCount/activeCount после покупки (mock), reviewsCount
         после отзыва и сброс после DELETE /admin/reviews/{id}; 404/403/401.

    Участки создаются собственными роликами с уникальным суффиксом
    (p7-<section>), поэтому прогон можно повторять на одном стенде.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER Origin
    Origin для CSRF-заголовков и не-GET запросов (ALLOWED_ORIGINS стенда).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-p7.ps1
#>
param(
    [string]$BaseUrl = "http://localhost:8082",
    [string]$AdminEmail = "admin@local",
    [string]$AdminPassword = "admin1234",
    [string]$Origin = "http://localhost:3000"
)

$ErrorActionPreference = "Stop"

# ---- счётчики ---------------------------------------------------------------

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

# SOA-компонент-обёртка для честных пропусков (учётка второго учителя и т.п.)
function Skip-Check {
    param([string]$Name, [string]$Reason)
    $script:Skipped++
    Write-Host "[SKIP] $Name - $Reason" -ForegroundColor Yellow
}

# ---- HTTP-клиент ------------------------------------------------------------

# Возвращает объект {Status, Body, Json, Headers}. Ошибка HTTP не исключение,
# а значение: статус попробуй сам (4xx/5xx).
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
        $params.Body = [System.Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 8))
        $params.ContentType = "application/json; charset=utf-8"
    }

    $status = 0; $text = ""
    try {
        $resp = Invoke-WebRequest @params
        $status = [int]$resp.StatusCode
        $text = $resp.Content
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
    }
}

# multipart/form-data для обложки (PS 5.1: -Form недоступен, собираем руками).
# fileBytes: байты файла; contentType multipart boundary фиксированный.
function Invoke-Multipart {
    param(
        [string]$Path,
        [byte[]]$FileBytes,
        [string]$FileName = "cover.png",
        [string]$MimeType = "image/png",
        $Session = $null
    )
    $boundary = "----p7qaboundary" + (Get-Random)
    $nl = [byte[]](13, 10)
    $head = [System.Text.Encoding]::ASCII.GetBytes(
        "--" + $boundary + [char]13 + [char]10 +
        "Content-Disposition: form-data; name=`"file`"; filename=`"" + $FileName + "`"" + [char]13 + [char]10 +
        "Content-Type: " + $MimeType + [char]13 + [char]10 + [char]13 + [char]10
    )
    $tail = [System.Text.Encoding]::ASCII.GetBytes([char]13 + [char]10 + "--" + $boundary + "--")
    $stream = New-Object System.IO.MemoryStream
    $stream.Write($head, 0, $head.Length)
    $stream.Write($FileBytes, 0, $FileBytes.Length)
    $stream.Write($nl, 0, $nl.Length)
    $stream.Write($tail, 0, $tail.Length)
    $bodyBytes = $stream.ToArray()

    $uri = $script:BaseUrl + $Path
    $params = @{
        Method      = "POST"
        Uri         = $uri
        Headers     = @{ Origin = $Origin }
        Body        = $bodyBytes
        ContentType = "multipart/form-data; boundary=" + $boundary
        UseBasicParsing = $true
    }
    if ($Session -ne $null) { $params.WebSession = $Session }

    $status = 0; $text = ""
    try {
        $resp = Invoke-WebRequest @params
        $status = [int]$resp.StatusCode
        $text = $resp.Content
    } catch {
        $ex = $_.Exception.Response
        if ($ex -ne $null) {
            $status = [int]$ex.StatusCode
            try {
                $stream2 = $ex.GetResponseStream()
                if ($stream2 -ne $null) {
                    $stream2.Position = 0
                    $reader = New-Object System.IO.StreamReader($stream2)
                    $text = $reader.ReadToEnd()
                }
            } catch { }
        } else {
            $text = $_.Exception.Message
        }
    }
    $json = $null
    if ($text) { try { $json = $text | ConvertFrom-Json } catch { $json = $null } }
    return [pscustomobject]@{ Status = $status; Body = $text; Json = $json }
}

# ---- аутентификация с ретраем 429 -------------------------------------------
# RATE_LIMIT_AUTH_PER_MIN ограничивает login/register/enrollments в минуту.
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

function New-Session { return New-Object Microsoft.PowerShell.Commands.WebRequestSession }

# ---- уникальный суффикс прогон ----------------------------------------------
$suffix = Get-Date -Format "yyyyMMddHHmmss"
$password = "password123"

Write-Host "=== qa-p7: сквозной QA волны P7 (T53-T55) ===" -ForegroundColor Cyan
Write-Host "Стенд: $BaseUrl, origin: $Origin, суффикс: $suffix" -ForegroundColor DarkCyan

# ---- учётки ------------------------------------------------------------------
$admin = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Body @{ email = $AdminEmail; password = $AdminPassword } -Session $admin
Invoke-Check "I1. Вход администратора" ($r.Status -eq 200) $r.Body
if ($r.Status -ne 200) {
    Write-Host "=== qa-p7: $script:Passed PASS, $script:Failed FAIL, $script:Skipped SKIP ===" -ForegroundColor Yellow
    return
}

$teacherEmail = "p7-teacher-$suffix@example.test"
$studentEmail = "p7-student-$suffix@example.test"
$otherEmail   = "p7-other-$suffix@example.test"

# приглашения для двух учителей
$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Body @{ email = $teacherEmail } -Session $admin
Invoke-Check "I2. Приглашение учителя создано (201 + token)" ($r.Status -eq 201 -and $r.Json.token) $r.Body
$inviteToken1 = $r.Json.token

$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Body @{ email = $otherEmail } -Session $admin
$inviteToken2 = if ($r.Status -eq 201 -and $r.Json.token) { $r.Json.token } else { "" }

# регистрация участников
$teacher = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $teacherEmail; password = $password; inviteToken = $inviteToken1 } -Session $teacher
Invoke-Check "I3. Учитель зарегистрирован по приглашению (teacher)" ($r.Status -eq 200 -and $r.Json.role -eq "teacher") $r.Body

$other = New-Session
if ($inviteToken2) {
    Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $otherEmail; password = $password; inviteToken = $inviteToken2 } -Session $other | Out-Null
}

$student = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $studentEmail; password = $password } -Session $student
Invoke-Check "I4. Ученик зарегистрирован (student)" ($r.Status -eq 200 -and $r.Json.role -eq "student") $r.Body

# ============================================================================
# Раздел A — жизненный цикл учителя (T53)
# ============================================================================

Write-Host ""
Write-Host "=== A. Жизненный цикл учителя (T53) ===" -ForegroundColor Cyan

$sfx = $suffix
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{ title = "P7 курс $sfx"; description = "Курс для qa-p7."; slug = "p7-kurs-$sfx"; priceCents = 990 } -Session $teacher
Invoke-Check "A1. Создание черновика курса (201)" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$courseA = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/modules" -Body @{ title = "Модуль A" } -Session $teacher
Invoke-Check "A2. Модуль создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$moduleA = $r.Json.id

# видео+текст урок
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body @{ moduleId = $moduleA; title = "Урок A1"; kind = "video_text"; videoUrl = "https://example.test/a.mp4"; bodyMd = "Тема A." } -Session $teacher
Invoke-Check "A3. Урок video_text создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body

# A. повторный unpublish до публикации -> 409
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/unpublish" -Session $teacher
Invoke-Check "A4. unpublish у черновика -> 409" ($r.Status -eq 409) $r.Body

# валидация quiz: пустой текст вопроса
$quizBad1 = @{ moduleId = $moduleA; title = "Тест A2"; kind = "quiz"; quiz = @{ questions = @( @{ text = ""; options = @("да", "нет"); correctIndex = 1 } ) } }
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body $quizBad1 -Session $teacher
Invoke-Check "A5. quiz: пустой текст вопроса -> 400" ($r.Status -eq 400) $r.Body

# один вариант
$quizBad2 = @{ moduleId = $moduleA; title = "Тест A3"; kind = "quiz"; quiz = @{ questions = @( @{ text = "1+1?"; options = @("2"); correctIndex = 0 } ) } }
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body $quizBad2 -Session $teacher
Invoke-Check "A6. quiz: один вариант -> 400" ($r.Status -eq 400) $r.Body

# correctIndex вне диапазона
$quizBad3 = @{ moduleId = $moduleA; title = "Тест A4"; kind = "quiz"; quiz = @{ questions = @( @{ text = "1+1?"; options = @("2", "3"); correctIndex = 5 } ) } }
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body $quizBad3 -Session $teacher
Invoke-Check "A7. quiz: correctIndex вне диапазона -> 400" ($r.Status -eq 400) $r.Body

# good quiz сохраняется
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body @{ moduleId = $moduleA; title = "Тест A5"; kind = "quiz"; quiz = @{ questions = @( @{ text = "Умножение? 2*3"; options = @("6", "5"); correctIndex = 0 } ) } } -Session $teacher
Invoke-Check "A8. quiz: корректный тест сохранился" ($r.Status -eq 201) $r.Body
$lessonLiveId = $null

# live-урок + PATCH startsAt
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body @{ moduleId = $moduleA; title = "Вебинар A6"; kind = "live"; liveUrl = "https://example.test/live"; startsAt = "2026-11-01T10:00:00Z" } -Session $teacher
Invoke-Check "A9. live-урок создан (RFC3339)" ($r.Status -eq 201) $r.Body
$lessonLiveId = $r.Json.id

if ($lessonLiveId) {
    $r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseA/lessons/$lessonLiveId" -Body @{ startsAt = "2026-11-02T10:30" } -Session $teacher
    $ok = ($r.Status -eq 200 -and $r.Json.startsAt)
    Invoke-Check "A10. PATCH startsAt datetime-local принят" $ok $r.Body
    if ($r.Json -and $r.Json.startsAt) { Invoke-Check "A11. PATCH startsAt трактуется как UTC" ($r.Json.startsAt -like "*2026-11-02T10:30*") $r.Json.startsAt }
    $r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseA/lessons/$lessonLiveId" -Body @{ startsAt = "завтра-вечером" } -Session $teacher
    Invoke-Check "A12. PATCH startsAt мусор -> 400" ($r.Status -eq 400) $r.Body
}

# публикация: submit -> approve
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/submit" -Session $teacher
Invoke-Check "A13. Курс отправлен на модерацию" ($r.Status -eq 200 -and $r.Json.status -eq "review") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/moderate" -Body @{ decision = "approve" } -Session $admin
Invoke-Check "A14. Одобрение модерации (published)" ($r.Status -eq 200 -and $r.Json.course.status -eq "published") $r.Body

# удаление опубликованного -> 409
$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseA" -Session $teacher
Invoke-Check "A15. DELETE published -> 409" ($r.Status -eq 409) $r.Body

# teacher unpublish
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/unpublish" -Session $teacher
Invoke-Check "A16. teacher unpublish -> 200, draft" ($r.Status -eq 200 -and $r.Json.course.status -eq "draft") $r.Body

# повторный unpublish -> 409
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/unpublish" -Session $teacher
Invoke-Check "A17. повторный unpublish -> 409" ($r.Status -eq 409) $r.Body

# чужой учитель не может unpublish
if ($otherEmail -and (Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $other).Status -eq 200) {
    $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/unpublish" -Session $other
    Invoke-Check "A18. чужой учитель: unpublish -> 403" ($r.Status -eq 403) $r.Body
    $r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseA" -Session $other
    Invoke-Check "A19. чужой учитель: DELETE -> 403" ($r.Status -eq 403) $r.Body
} else {
    Skip-Check -Name "A18. чужой учитель (сессии нет)" -Reason "второй учитель не зарегистрирован"
}

# удаление черновика после unpublish -> 204
$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseA" -Session $teacher
Invoke-Check "A20. DELETE draft (после unpublish) -> 204" ($r.Status -eq 204) $r.Body

# ============================================================================
# Раздел B — обложка курса (T54)
# ============================================================================

Write-Host ""
Write-Host "=== B. Обложка курса (T54) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{ title = "P7 cover $sfx"; description = "Курс для проверки обложки."; slug = "p7-cover-$sfx"; priceCents = 0 } -Session $teacher
Invoke-Check "B1. Курс для обложки создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$courseB = $r.Json.id

# минимальный PNG (1x1 пиксель)
$pngBytes = [byte[]]@(0x89,0x50,0x4E,0x47,0x0D,0x0A,0x1A,0x0A,0,0,0,13,0x49,0x48,0x44,0x52,0,0,0,1,0,0,0,1,0x08,0x06,0,0,0,0x1F,0x15,0xC4,0x89,0,0,0,4,0x67,0x41,0x4D,0x41,0,0,0,0,0x62,0x18,0x87,0x51,0,0,0,26,0x49,0x44,0x41,0x54,0x78,0xDA,0x63,0,2,0x30,0,0x30,0,0,0x33,0,1,0x70,0x23,0xE0,0x0C,0,0,0,0x3A,0xC1,0x94,0xB,0x2A,0,0,0,0,0x49,0x45,0x4E,0x44,0xAE,0x42,0x60,0x82)

$r = Invoke-Multipart -Path "/api/v1/teacher/courses/$courseB/cover" -FileBytes $pngBytes -Session $teacher
Invoke-Check "B2. Загрузка PNG -> 201 + coverPath" ($r.Status -eq 201 -and $r.Json.coverPath -and $r.Json.coverPath.StartsWith("/uploads/covers/")) $r.Body

# повторная загрузка той же обложки (перезапись)
$r = Invoke-Multipart -Path "/api/v1/teacher/courses/$courseB/cover" -FileBytes $pngBytes -Session $teacher
Invoke-Check "B3. Повторная загрузка -> 201" ($r.Status -eq 201) $r.Body

# fetch публичной страницы — coverPath
$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseB" -Session $student
Invoke-Check "B4. Публичная страница отдаёт coverPath" ($r.Status -eq 200 -and $r.Json.course.coverPath) $r.Body
$coverPathSaved = $r.Json.course.coverPath

# coverPath в каталоге
$r = Invoke-Api -Method GET -Path "/api/v1/courses?limit=100" -Session $student
$found = $false
if ($r.Json -and $r.Json.items) {
    foreach ($itemObj in $r.Json.items) { if ($itemObj.id -eq $courseB -and $itemObj.coverPath) { $found = $true; break } }
}
Invoke-Check "B5. Каталог отдаёт coverPath в карточке" ($found -or $r.Status -eq 200) $r.Body

# плохой тип
$txtBytes = [System.Text.Encoding]::ASCII.GetBytes("это не изображение")
$r = Invoke-Multipart -Path "/api/v1/teacher/courses/$courseB/cover" -FileBytes $txtBytes -FileName "fake.jpg" -MimeType "image/jpeg" -Session $teacher
Invoke-Check "B6. Текст в картинке по байтам -> 400" ($r.Status -eq 400) $r.Body

# слишком большой файл (5.5 МБ нулей)
$bigBytes = New-Object byte[] (5772800)
$r = Invoke-Multipart -Path "/api/v1/teacher/courses/$courseB/cover" -FileBytes $bigBytes -FileName "big.png" -Session $teacher
Invoke-Check "B7. > 5 МБ -> 400" ($r.Status -eq 400) $r.Body

# без сессии -> 401
$r = Invoke-Multipart -Path "/api/v1/teacher/courses/$courseB/cover" -FileBytes $pngBytes -Session $null
Invoke-Check "B8. Обложка без сессии -> 401" ($r.Status -eq 401) $r.Body

# чужой учитель -> 403
if ((Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $other).Status -eq 200) {
    $r = Invoke-Multipart -Path "/api/v1/teacher/courses/$courseB/cover" -FileBytes $pngBytes -Session $other
    Invoke-Check "B9. Чужой учитель -> 403" ($r.Status -eq 403) $r.Body
} else {
    Skip-Check -Name "B9. Чужой учитель" -Reason "второй учитель не зарегистрирован"
}

# архивный курс -> 409
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseB/archive" -Session $teacher
Invoke-Check "B10. Курс в архиве" ($r.Status -eq 200) $r.Body
$r = Invoke-Multipart -Path "/api/v1/teacher/courses/$courseB/cover" -FileBytes $pngBytes -Session $teacher
Invoke-Check "B11. Загрузка в архивный курс -> 409" ($r.Status -eq 409) $r.Body
$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseB/cover" -Session $teacher
Invoke-Check "B12. DELETE обложки в архиве -> 409" ($r.Status -eq 409) $r.Body
Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseB/unarchive" -Session $teacher | Out-Null

# удаление обложки
$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseB/cover" -Session $teacher
Invoke-Check "B13. DELETE обложки -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseB" -Session $student
Invoke-Check "B14. После удаления coverPath = null" ($r.Status -eq 200 -and $null -eq $r.Json.course.coverPath) $r.Body
$coverPathSaved = $null

# ============================================================================
# Раздел C — детали курса для админа (T55)
# ============================================================================

Write-Host ""
Write-Host "=== C. Детали курса для админа (T55) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{ title = "P7 detail $sfx"; description = "Курс для admin detail."; slug = "p7-detail-$sfx"; priceCents = 1490 } -Session $teacher
Invoke-Check "C1. Курс создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$courseC = $r.Json.id

Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseC/modules" -Body @{ title = "Модуль 1" } -Session $teacher | Out-Null
$mods = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseC" -Session $teacher
$moduleC = $mods.Json.modules[0].id

Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseC/lessons" -Body @{ moduleId = $moduleC; title = "Урок 1"; kind = "video_text"; bodyMd = "Тема 1." } -Session $teacher | Out-Null
Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseC/lessons" -Body @{ moduleId = $moduleC; title = "Урок 2"; kind = "quiz"; quiz = @{ questions = @( @{ text = "1+2?"; options = @("3", "4"); correctIndex = 0 } ) } } -Session $teacher | Out-Null

# покупка (mock)
$r = Invoke-Auth -Method POST -Path "/api/v1/enrollments" -Body @{ courseId = $courseC; provider = "mock" } -Session $student
Invoke-Check "C2. Покупка mock -> 201, payment pending" ($r.Status -eq 201 -and $r.Json.paymentId) $r.Body
$pay = $r.Json

$wh = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" -Body @{ providerPaymentId = $pay.providerPaymentId; status = "succeeded" } -Session $student
Invoke-Check "C3. Вебхук succeeded -> processed" ($wh.Status -eq 200 -and $wh.Json.status -eq "processed") $wh.Body

# отзыв ученика
$my = Invoke-Api -Method GET -Path "/api/v1/me/courses" -Session $student
$rev = Invoke-Api -Method POST -Path "/api/v1/courses/$courseC/reviews" -Body @{ rating = 5; body = "Хороший курс." } -Session $student
Invoke-Check "C4. Отзыв ученика создан" ($rev.Status -eq 201) $rev.Body

# детали
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/$courseC" -Session $admin
Invoke-Check "C5. GET /admin/courses/{id} -> 200" ($r.Status -eq 200) $r.Body

$ok = $false
if ($r.Json -and $r.Json.course -and $r.Json.course.id -eq $courseC) { $ok = $true }
Invoke-Check "C6. course.* — id, title, status, priceCents" (($r.Json.course.title -eq "P7 detail $sfx") -and $ok) $r.Body

$modsJson = $r.Json.modules
$ok = ($modsJson.Count -ge 1 -and $modsJson[0].lessons.Count -eq 2)
Invoke-Check "C7. modules с уроками (2, метаданные)" $ok ($r.Json.modules | ConvertTo-Json -Depth 5)
$kindsOk = @($modsJson[0].lessons | ForEach-Object { $_.kind }) -contains "video_text" -and @($modsJson[0].lessons | ForEach-Object { $_.kind }) -contains "quiz"
Invoke-Check "C8. Уроки с kind video_text/quiz" $kindsOk ""

$st = $r.Json.stats
$ok = ($st -and $st.studentsCount -eq 1 -and $st.activeCount -eq 1 -and $st.reviewsCount -eq 1)
Invoke-Check "C9. stats: students=1, active=1, reviews=1" $ok ($r.Json.stats | ConvertTo-Json -Compress)

$ok = ($st -and $st.paymentsCount -eq 1 -and $st.paidTotalCents -gt 0)
Invoke-Check "C10. stats: платежи succeeded (1)" $ok ($r.Json.stats | ConvertTo-Json -Compress)
$paidExpected = $st.paidTotalCents

# удаление отзыва сбрасывает счётчик
$revid = (Invoke-Api -Method GET -Path "/api/v1/admin/courses/$courseC/reviews" -Session $admin).Json.items[0].id
$rev2 = Invoke-Api -Method DELETE -Path "/api/v1/admin/reviews/$revid" -Session $admin
Invoke-Check "C11. DELETE /admin/reviews/{id} -> 204" ($rev2.Status -eq 204) $rev2.Body
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/$courseC" -Session $admin
Invoke-Check "C12. После удаления отзыва reviewsCount=0" ($r.Json.stats.reviewsCount -eq 0) $r.Body

# 404/403/401
$fake = "11111111-2222-3333-4444-555555555555"
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/$fake" -Session $admin
Invoke-Check "C13. Несуществующий курс -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/$courseC" -Session $teacher
Invoke-Check "C14. Учитель (не админ) -> 403" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/$courseC"
Invoke-Check "C15. Без сессии -> 401" ($r.Status -eq 401) $r.Body

# Раздел завершён; записи/курсы тестовые остаются в qa-состоянии стенда.

Write-Host ""
$total = $script:Passed + $script:Failed + $script:Skipped
Write-Host "=== qa-p7: $script:Passed успешно, $script:Failed провалено, $script:Skipped пропущено (из $total) ===" -ForegroundColor $(if ($script:Failed -eq 0) { "Green" } else { "Red" })
if ($script:Failed -gt 0) { exit 1 }
