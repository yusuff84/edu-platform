<#
.SYNOPSIS
    qa-p10.ps1 — сквозной QA-прогон волны P10 (задачи T83–T94).

.DESCRIPTION
    Проверяет на живом стенде API-часть волны:
      A. Глобальная модерация отзывов (T83): фильтры, поиск, сортировка,
         пагинация, удаление и пересчёт рейтинга курса.
      B. Глобальная модерация Q&A (T85): фильтры, поиск, ответы внутри строки,
         удаление чужих вопросов/ответов супер-админом.
      C. Редактирование курса супер-админом (T87): правка published/review
         курса, teacher-guard, force publish, архив, moderationComment.
      D. Глобальные промокоды (T89): журнал, фильтры, частичный PATCH,
         валидация, архивный курс.
      E. Сессии пользователя (T93): список активных, принудительный выход,
         изоляция чужих сессий.

    Все сущности создаются с уникальным суффиксом (p10-*), прогон можно
    повторять на одном стенде.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER Origin
    Origin для CSRF-заголовков и не-GET запросов (ALLOWED_ORIGINS стенда).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-p10.ps1
#>
param(
    [string]$BaseUrl = "http://localhost:8082",
    [string]$AdminEmail = "admin@local",
    [string]$AdminPassword = "admin1234",
    [string]$Origin = "http://localhost:3001"
)

$ErrorActionPreference = "Stop"

# ---- счётчики ---------------------------------------------------------------

$script:Passed = 0
$script:Failed = 0

function Invoke-Check {
    param([string]$Name, [bool]$Ok, [string]$Detail = "")
    if ($Ok) {
        $script:Passed++
        Write-Host "[PASS] $Name" -ForegroundColor Green
    } else {
        $script:Failed++
        $short = $Detail
        if ($short.Length -gt 260) { $short = $short.Substring(0, 260) + "..." }
        Write-Host "[FAIL] $Name - $short" -ForegroundColor Red
    }
}

# ---- HTTP-клиент ------------------------------------------------------------

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [object]$Body = $null,
        $Session = $null,
        [hashtable]$Headers = @{}
    )
    $uri = $script:BaseUrl + $Path

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
        $params.Body = [System.Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 10))
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
    return [pscustomobject]@{ Status = $status; Body = $text; Json = $json }
}

# Аутентификация с ретраем 429 (лимитер login/register/enrollments).
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

function ItemCount {
    param($Envelope)
    if ($Envelope -eq $null -or $Envelope.items -eq $null) { return 0 }
    return @($Envelope.items).Count
}

$suffix = Get-Date -Format "yyyyMMddHHmmss"
$password = "password123"

Write-Host "=== qa-p10: сквозной QA волны P10 (T83–T94) ===" -ForegroundColor Cyan
Write-Host "Стенд: $BaseUrl, origin: $Origin, суффикс: $suffix" -ForegroundColor DarkCyan

# ---- учётки ------------------------------------------------------------------

$admin = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Body @{ email = $AdminEmail; password = $AdminPassword } -Session $admin
Invoke-Check "I1. Вход администратора" ($r.Status -eq 200) $r.Body
if ($r.Status -ne 200) {
    Write-Host "=== qa-p10: $script:Passed PASS, $script:Failed FAIL ===" -ForegroundColor Yellow
    return
}

$teacherEmail = "p10-teacher-$suffix@example.test"
$studentEmail = "p10-student-$suffix@example.test"

$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Body @{ email = $teacherEmail } -Session $admin
Invoke-Check "I2. Приглашение учителя создано" ($r.Status -eq 201 -and $r.Json.token) $r.Body
$inviteToken = $r.Json.token

$teacher = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $teacherEmail; password = $password; inviteToken = $inviteToken } -Session $teacher
Invoke-Check "I3. Учитель зарегистрирован" ($r.Status -eq 201 -and $r.Json.role -eq "teacher") $r.Body

$student = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $studentEmail; password = $password } -Session $student
Invoke-Check "I4. Ученик зарегистрирован" ($r.Status -eq 201) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $student
$studentId = $r.Json.id
Invoke-Check "I5. id ученика получен" ($studentId -ne $null) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $teacher
$teacherId = $r.Json.id

$anon = New-Session

# ---- курсы -------------------------------------------------------------------

# PA: бесплатный опубликованный курс для отзывов, Q&A, промокодов и правок.
$slugPA = "p10-main-$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{
    title = "P10 main $suffix"; description = "QA P10 main course."; slug = $slugPA
    priceCents = 0; category = "design"
} -Session $teacher
Invoke-Check "S1. Основной курс создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$courseA = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/modules" -Body @{ title = "P10 module A" } -Session $teacher
Invoke-Check "S2. Модуль создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$moduleA = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body @{
    moduleId = $moduleA; title = "P10 lesson A"; kind = "video_text"
    videoUrl = "https://example.test/p10-main.mp4"; bodyMd = "Main course lesson body."
} -Session $teacher
Invoke-Check "S3. Урок создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$lessonA = $r.Json.id

# T87: force publish из draft, детали админа и moderationComment.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/$courseA" -Session $admin
Invoke-Check "S4. Детали курса для админа (moderationComment = '')" ($r.Status -eq 200 -and $r.Json.course.moderationComment -eq "") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/publish" -Session $admin
Invoke-Check "S5. Force publish из draft -> published" ($r.Status -eq 200 -and $r.Json.course.status -eq "published") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/publish" -Session $admin
Invoke-Check "S6. Повторный force publish идемпотентен" ($r.Status -eq 200 -and $r.Json.course.status -eq "published") $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPA" -Session $anon
Invoke-Check "S7. Курс виден публично" ($r.Status -eq 200 -and $r.Json.course.status -eq "published" -and @($r.Json.modules).Count -eq 1) $r.Body

$r = Invoke-Auth -Method POST -Path "/api/v1/enrollments" -Body @{ courseId = $courseA; provider = "mock" } -Session $student
Invoke-Check "S8. Ученик записан на бесплатный курс" ($r.Status -eq 201 -and $r.Json.enrollmentActive -eq $true) $r.Body

# T87: админ правит содержимое published-курса, teacher — по-прежнему 409.
$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseA/lessons/$lessonA" -Body @{ title = "P10 lesson A (admin edit)" } -Session $admin
Invoke-Check "S9. Админ правит урок published-курса" ($r.Status -eq 200 -and $r.Json.title -eq "P10 lesson A (admin edit)") $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseA/lessons/$lessonA" -Body @{ title = "P10 lesson A (teacher edit)" } -Session $teacher
Invoke-Check "S10. Teacher не правит published-курс (409)" ($r.Status -eq 409) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPA" -Session $anon
$lessonPublic = @($r.Json.modules)[0].lessons[0]
Invoke-Check "S11. Правка админа видна публично" ($lessonPublic.title -eq "P10 lesson A (admin edit)") $r.Body

# PB: черновик с промокодом; после force publish архивируется для негативных проверок.
$slugPB = "p10-archive-$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{
    title = "P10 archive $suffix"; description = "QA P10 archive course."; slug = $slugPB
    priceCents = 0; category = "design"
} -Session $teacher
$courseB = $r.Json.id
Invoke-Check "S12. Второй курс создан" ($r.Status -eq 201 -and $courseB) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseB/modules" -Body @{ title = "P10 module B" } -Session $teacher
$moduleB = $r.Json.id
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseB/lessons" -Body @{
    moduleId = $moduleB; title = "P10 lesson B"; kind = "video_text"
    videoUrl = "https://example.test/p10-archive.mp4"; bodyMd = "Archive lesson body."
} -Session $teacher
$lessonB = $r.Json.id
Invoke-Check "S13. Модуль и урок второго курса созданы" ($moduleB -and $lessonB) $r.Body

$promoCodeB = "P10ARC$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseB/promo-codes" -Body @{
    code = $promoCodeB; discountPct = 15; maxUses = 5
} -Session $teacher
$promoB = $r.Json.promoCode.id
Invoke-Check "S14. Промокод архивного курса создан" ($r.Status -eq 201 -and $promoB) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseB/publish" -Session $admin
Invoke-Check "S15. Force publish второго курса" ($r.Status -eq 200 -and $r.Json.course.status -eq "published") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseB/archive" -Session $admin
Invoke-Check "S16. Курс переведён в архив" ($r.Status -eq 200 -and $r.Json.course.status -eq "archived") $r.Body

# ---- A. Глобальная модерация отзывов (T83) -----------------------------------

Write-Host ""
Write-Host "=== A. Глобальная модерация отзывов (T83) ===" -ForegroundColor Cyan

$reviewBody = "P10 review ${suffix}: отличный курс."
$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/reviews" -Body @{ rating = 5; body = $reviewBody } -Session $student
$reviewId = $r.Json.id
Invoke-Check "A1. Отзыв создан" ($r.Status -eq 201 -and $reviewId) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/reviews?courseId=$courseA&rating=5&q=p10&sort=rating&dir=desc&page=1&limit=10" -Session $admin
$reviewRow = @($r.Json.items | Where-Object { $_.id -eq $reviewId })[0]
Invoke-Check "A2. Отзыв в глобальном журнале с фильтрами" ($r.Status -eq 200 -and $r.Json.total -ge 1 -and $reviewRow.courseTitle -like "P10 main*" -and $reviewRow.studentEmail -eq $studentEmail) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/reviews?rating=6" -Session $admin
Invoke-Check "A3. rating=6 -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/reviews?sort=body" -Session $admin
Invoke-Check "A4. Неизвестная сортировка -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/reviews?from=2026-13-01" -Session $admin
Invoke-Check "A5. Неверная дата -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/reviews" -Session $anon
Invoke-Check "A6. Гость не видит журнал отзывов (401)" ($r.Status -eq 401) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/reviews" -Session $teacher
Invoke-Check "A7. Teacher не видит журнал отзывов (403)" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/reviews/$reviewId" -Session $admin
Invoke-Check "A8. Админ удаляет отзыв (204)" ($r.Status -eq 204) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseA/reviews" -Session $anon
Invoke-Check "A9. Рейтинг пересчитан после удаления" ($r.Status -eq 200 -and $r.Json.total -eq 0 -and $r.Json.ratingsCount -eq 0) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/reviews?courseId=$courseA&q=p10" -Session $admin
Invoke-Check "A10. Удалённого отзыва нет в журнале" ($r.Status -eq 200 -and $r.Json.total -eq 0) $r.Body

# ---- B. Глобальная модерация Q&A (T85) ---------------------------------------

Write-Host ""
Write-Host "=== B. Глобальная модерация Q&A (T85) ===" -ForegroundColor Cyan

$questionBody = "P10 question ${suffix}: как работает превью урока?"
$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/questions" -Body @{ lessonId = $lessonA; body = $questionBody } -Session $student
$questionId = $r.Json.question.id
Invoke-Check "B1. Вопрос создан" ($r.Status -eq 201 -and $questionId) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/questions/$questionId/answers" -Body @{ body = "P10 answer ${suffix}: превью доступно без записи." } -Session $teacher
$answerId = $r.Json.answer.id
Invoke-Check "B2. Ответ преподавателя создан" ($r.Status -eq 201 -and $answerId) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/questions?courseId=$courseA&lessonId=$lessonA&q=p10&page=1&limit=10" -Session $admin
$questionRow = @($r.Json.items | Where-Object { $_.id -eq $questionId })[0]
Invoke-Check "B3. Вопрос в глобальном журнале с ответом" ($r.Status -eq 200 -and $r.Json.total -ge 1 -and $questionRow.courseTitle -like "P10 main*" -and $questionRow.lessonTitle -eq "P10 lesson A (admin edit)" -and @($questionRow.answers).Count -eq 1) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/questions?courseId=not-a-uuid" -Session $admin
Invoke-Check "B4. Неверный courseId -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/questions?limit=101" -Session $admin
Invoke-Check "B5. limit=101 -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/questions" -Session $anon
Invoke-Check "B6. Гость не видит журнал Q&A (401)" ($r.Status -eq 401) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/questions" -Session $teacher
Invoke-Check "B7. Teacher не видит журнал Q&A (403)" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$courseA/questions/$questionId/answers/$answerId" -Session $admin
Invoke-Check "B8. Админ удаляет чужой ответ (204)" ($r.Status -eq 204) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/questions?courseId=$courseA&q=p10" -Session $admin
$questionRow = @($r.Json.items | Where-Object { $_.id -eq $questionId })[0]
Invoke-Check "B9. В строке вопроса ответов не осталось" ($r.Status -eq 200 -and $questionRow -and @($questionRow.answers).Count -eq 0) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$courseA/questions/$questionId" -Session $admin
Invoke-Check "B10. Админ удаляет чужой вопрос (204)" ($r.Status -eq 204) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/questions?courseId=$courseA&q=p10" -Session $admin
Invoke-Check "B11. Удалённого вопроса нет в журнале" ($r.Status -eq 200 -and $r.Json.total -eq 0) $r.Body

# ---- C. Правка курса супер-админом: архив (T87) ------------------------------

Write-Host ""
Write-Host "=== C. Архивный курс: запрет правок и force publish (T87) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$courseB/lessons/$lessonB" -Body @{ title = "Archived must not change" } -Session $admin
Invoke-Check "C1. Админ не правит архивный курс (409)" ($r.Status -eq 409) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseB/publish" -Session $admin
Invoke-Check "C2. Force publish архивного курса -> 409" ($r.Status -eq 409) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/00000000-0000-0000-0000-000000000000/publish" -Session $admin
Invoke-Check "C3. Force publish несуществующего курса -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/publish" -Session $teacher
Invoke-Check "C4. Teacher не может force publish (403)" ($r.Status -eq 403) $r.Body

# ---- D. Глобальные промокоды (T89) ------------------------------------------

Write-Host ""
Write-Host "=== D. Глобальные промокоды (T89) ===" -ForegroundColor Cyan

$promoCodeA = "P10MAIN$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/promo-codes" -Body @{
    code = $promoCodeA; discountPct = 20; maxUses = 10
} -Session $teacher
$promoA = $r.Json.promoCode.id
Invoke-Check "D1. Промокод создан" ($r.Status -eq 201 -and $promoA) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/promo-codes?courseId=$courseA&q=$promoCodeA&active=true&page=1&limit=10" -Session $admin
$promoRow = @($r.Json.items | Where-Object { $_.id -eq $promoA })[0]
Invoke-Check "D2. Промокод в глобальном журнале (курс, поиск, active)" ($r.Status -eq 200 -and $r.Json.total -eq 1 -and $promoRow.courseTitle -like "P10 main*" -and $promoRow.discountPct -eq 20) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/promo-codes/$promoA" -Body @{ discountPct = 25; active = $false; expiresAt = $null } -Session $admin
Invoke-Check "D3. Частичный PATCH: скидка, активность, очистка срока" ($r.Status -eq 200 -and $r.Json.promoCode.discountPct -eq 25 -and $r.Json.promoCode.active -eq $false -and $r.Json.promoCode.expiresAt -eq $null) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/promo-codes/$promoA" -Body @{ discountPct = 0 } -Session $admin
Invoke-Check "D4. discountPct=0 -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/promo-codes/$promoA" -Body @{ expiresAt = "2020-01-01T00:00:00Z" } -Session $admin
Invoke-Check "D5. Прошедший expiresAt -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/promo-codes/$promoA" -Body @{ unknownField = 1 } -Session $admin
Invoke-Check "D6. Неизвестное поле -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/promo-codes/00000000-0000-0000-0000-000000000000" -Body @{ active = $true } -Session $admin
Invoke-Check "D7. Несуществующий промокод -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/promo-codes/$promoB" -Body @{ active = $true } -Session $admin
Invoke-Check "D8. Промокод архивного курса -> 409" ($r.Status -eq 409) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/promo-codes?active=maybe" -Session $admin
Invoke-Check "D9. active=maybe -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/promo-codes" -Session $anon
Invoke-Check "D10. Гость не видит журнал промокодов (401)" ($r.Status -eq 401) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/promo-codes" -Session $teacher
Invoke-Check "D11. Teacher не видит журнал промокодов (403)" ($r.Status -eq 403) $r.Body

# ---- E. Сессии и принудительный выход (T93) ----------------------------------

Write-Host ""
Write-Host "=== E. Сессии и принудительный выход (T93) ===" -ForegroundColor Cyan

$student2 = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Body @{ email = $studentEmail; password = $password } -Session $student2
Invoke-Check "E0. Второй вход ученика создаёт ещё одну сессию" ($r.Status -eq 200 -and $r.Json.role -eq "student") $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/users/$studentId/sessions" -Session $admin
$sessions = @($r.Json.items)
$newestFirst = $true
if ($sessions.Count -ge 2) {
    for ($i = 1; $i -lt $sessions.Count; $i++) {
        if ([datetime]$sessions[$i - 1].createdAt -lt [datetime]$sessions[$i].createdAt) {
            $newestFirst = $false
        }
    }
}
Invoke-Check "E1. Админ видит активные сессии ученика (новые первыми)" ($r.Status -eq 200 -and $sessions.Count -ge 2 -and $newestFirst) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/users/$studentId/sessions" -Session $teacher
Invoke-Check "E2. Teacher не видит сессии (403)" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/users/00000000-0000-0000-0000-000000000000/sessions" -Session $admin
Invoke-Check "E3. Несуществующий пользователь -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/users/$studentId/sessions" -Session $anon
Invoke-Check "E4. Гость не видит сессии (401)" ($r.Status -eq 401) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/users/$studentId/sessions" -Session $admin
Invoke-Check "E5. Все сессии ученика отозваны (204)" ($r.Status -eq 204) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $student
Invoke-Check "E6. Первая сессия ученика потеряла доступ" ($r.Status -eq 401) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $student2
Invoke-Check "E6b. Вторая сессия ученика тоже потеряла доступ" ($r.Status -eq 401) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $admin
Invoke-Check "E7. Сессия админа не затронута" ($r.Status -eq 200 -and $r.Json.role -eq "admin") $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $teacher
Invoke-Check "E8. Сессия учителя не затронута" ($r.Status -eq 200 -and $r.Json.role -eq "teacher") $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/users/$studentId/sessions" -Session $admin
Invoke-Check "E9. После отзыва список пуст" ($r.Status -eq 200 -and (ItemCount $r.Json) -eq 0) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/users/$studentId/sessions" -Session $admin
Invoke-Check "E10. Повторный отзыв при нуле сессий — 204" ($r.Status -eq 204) $r.Body

# ---- Итоги -------------------------------------------------------------------

Write-Host ""
$color = if ($script:Failed -eq 0) { "Green" } else { "Yellow" }
Write-Host "=== qa-p10: $script:Passed PASS, $script:Failed FAIL ===" -ForegroundColor $color
if ($script:Failed -gt 0) { exit 1 }
