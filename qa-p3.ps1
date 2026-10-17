<#
.SYNOPSIS
    qa-p3.ps1 - сквозной QA-прогон раздела P3 (задачи T21-T30).

.DESCRIPTION
    Дополняет qa-abc.ps1 (базовый сквозной сценарий 49 проверок) проверкой
    того, что появилось в волне P3, на живом стенде через API:

      D. Конструктор курса (T21): перенос урока между модулями, отзыв
         с модерации, копирование курса, архив и возврат из архива.
      E. Ученики курса (T22): список записавшихся с прогрессом и статистика
         курса для преподавателя.
      F. Кабинет ученика (T23): серверный прогресс, профиль, история платежей.
      G. Супер-админ (T24/T25): управление пользователями (роль, блокировка,
         удаление), сводка платформы, все курсы, отзывы, все записи.

    Сценарий создаёт собственные сущности с уникальной меткой времени, поэтому
    его можно гонять повторно на том же стенде.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER SkipWeb
    Не проверять доступность фронтенда.

.EXAMPLE
    docker compose up -d --build db api web runner
    powershell -File qa-p3.ps1 -AdminEmail admin@local -AdminPassword admin1234
    docker compose down

.NOTES
    Написан под PowerShell 5.1. CSRF: у не-GET-запросов выставляется Origin,
    входящий в ALLOWED_ORIGINS стенда (по умолчанию http://localhost:3000).
#>
[CmdletBinding()]
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
# а статус + тело: проверяем и 4xx/5xx.
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

# =============================================================================
$stamp = (Get-Date).ToString("yyyyMMddHHmmss") + "-" + (Get-Random -Maximum 9999)
$teacherEmail  = "p3-teacher-$stamp@example.test"
$teacher2Email = "p3-teacher2-$stamp@example.test"
$studentEmail  = "p3-student-$stamp@example.test"
$password      = "password123"

Write-Host "=== qa-p3: сквозной QA раздела P3 (T21-T30) ===" -ForegroundColor Cyan
Write-Host "API: $BaseUrl | Origin: $Origin | Метка: $stamp"
Write-Host "Преподаватель: $teacherEmail"
Write-Host "Второй преподаватель: $teacher2Email"
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
Write-Host "--- Подготовка: админ, приглашения, преподаватели ---" -ForegroundColor Cyan

$adminSession = New-Session
$r = Invoke-Api -Method POST -Path "/api/v1/auth/login" -Session $adminSession `
    -Body @{ email = $AdminEmail; password = $AdminPassword }
Invoke-Check "P1. Вход админа -> 200" ($r.Status -eq 200) $r.Body

$adminId = ""
if ($r.Status -eq 200 -and $r.Json -ne $null) { $adminId = [string]$r.Json.id }

# Приглашения: роль teacher выдаётся только по invite-токену.
$inviteToken1 = ""
$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Session $adminSession `
    -Body @{ email = $teacherEmail }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $inviteToken1 = [string]$r.Json.token }
Invoke-Check "P2. Приглашение преподавателя -> 201 с токеном" `
    ($r.Status -eq 201 -and $inviteToken1 -ne "") $r.Body

$inviteToken2 = ""
$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Session $adminSession `
    -Body @{ email = $teacher2Email }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $inviteToken2 = [string]$r.Json.token }
Invoke-Check "P3. Приглашение второго преподавателя -> 201" `
    ($r.Status -eq 201 -and $inviteToken2 -ne "") $r.Body

$teacherSession = New-Session
$teacherId = ""
$r = Invoke-Api -Method POST -Path "/api/v1/auth/register" -Session $teacherSession `
    -Body @{ email = $teacherEmail; password = $password; inviteToken = $inviteToken1 }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $teacherId = [string]$r.Json.id }
Invoke-Check "P4. Регистрация преподавателя -> 201, role=teacher" `
    ($r.Status -eq 201 -and $r.Json.role -eq "teacher") $r.Body

$teacher2Session = New-Session
$r = Invoke-Api -Method POST -Path "/api/v1/auth/register" -Session $teacher2Session `
    -Body @{ email = $teacher2Email; password = $password; inviteToken = $inviteToken2 }
Invoke-Check "P5. Регистрация второго преподавателя -> 201" `
    ($r.Status -eq 201 -and $r.Json.role -eq "teacher") $r.Body

$studentSession = New-Session
$studentId = ""
$r = Invoke-Api -Method POST -Path "/api/v1/auth/register" -Session $studentSession `
    -Body @{ email = $studentEmail; password = $password }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $studentId = [string]$r.Json.id }
Invoke-Check "P6. Регистрация ученика -> 201, role=student" `
    ($r.Status -eq 201 -and $r.Json.role -eq "student") $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- D. Конструктор курса (T21) ---" -ForegroundColor Cyan

# D1. Курс + два модуля + три урока.
$courseID = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
    -Body @{ title = "P3 курс $stamp"; description = "Сквозной курс волны P3."; priceCents = 150000 }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $courseID = [string]$r.Json.id }
Invoke-Check "D1. Создание курса -> 201" ($r.Status -eq 201 -and $courseID -ne "") $r.Body

$moduleA = ""; $moduleB = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/modules" -Session $teacherSession `
    -Body @{ title = "Модуль A" }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $moduleA = [string]$r.Json.id }
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/modules" -Session $teacherSession `
    -Body @{ title = "Модуль B" }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $moduleB = [string]$r.Json.id }
Invoke-Check "D2. Два модуля созданы -> 201" `
    ($moduleA -ne "" -and $moduleB -ne "") $r.Body

$lessonVideoA = ""; $lessonQuizA = ""; $lessonVideoB = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons" -Session $teacherSession `
    -Body @{ moduleId = $moduleA; title = "Видео в A"; kind = "video_text"; bodyMd = "## Текст урока" }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $lessonVideoA = [string]$r.Json.id }
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons" -Session $teacherSession `
    -Body @{ moduleId = $moduleA; title = "Квиз в A"; kind = "quiz";
             quiz = @{ questions = @(
                 @{ id = "q1"; text = "2+2?"; options = @("3", "4"); correct = 1 },
                 @{ id = "q2"; text = "Столица РФ?"; options = @("Москва", "Питер"); correct = 0 }) } }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $lessonQuizA = [string]$r.Json.id }
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons" -Session $teacherSession `
    -Body @{ moduleId = $moduleB; title = "Видео в B"; kind = "video_text"; bodyMd = "Урок в модуле B" }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $lessonVideoB = [string]$r.Json.id }
Invoke-Check "D3. Три урока созданы -> 201" `
    ($lessonVideoA -ne "" -and $lessonQuizA -ne "" -and $lessonVideoB -ne "") $r.Body

# D4. Перенос урока A -> B.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons/$lessonVideoA/move" `
    -Session $teacherSession -Body @{ moduleId = $moduleB }
$movedOk = ($r.Status -eq 200 -and $r.Json.lesson -ne $null -and [string]$r.Json.lesson.moduleId -eq $moduleB)
Invoke-Check "D4. Перенос урока в другой модуль -> 200" $movedOk $r.Body

# D5. Перенос в несуществующий модуль -> 400.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons/$lessonQuizA/move" `
    -Session $teacherSession -Body @{ moduleId = "00000000-0000-0000-0000-0000000000ff" }
Invoke-Check "D5. Перенос в чужой модуль -> 400" ($r.Status -eq 400) $r.Body

# D6. Пустой moduleId -> 400.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons/$lessonQuizA/move" `
    -Session $teacherSession -Body @{ moduleId = "" }
Invoke-Check "D6. Пустой moduleId -> 400" ($r.Status -eq 400) $r.Body

# D7. Отправка на модерацию.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/submit" -Session $teacherSession
Invoke-Check "D7. Submit -> 200, status=review" `
    ($r.Status -eq 200 -and $r.Json.status -eq "review") $r.Body

# D8. В review структуру менять нельзя -> 409.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons/$lessonQuizA/move" `
    -Session $teacherSession -Body @{ moduleId = $moduleB }
Invoke-Check "D8. Перенос на модерации -> 409" ($r.Status -eq 409) $r.Body

# D9. Отзыв с модерации.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/withdraw" -Session $teacherSession
Invoke-Check "D9. Withdraw -> 200, status=draft" `
    ($r.Status -eq 200 -and $r.Json.course.status -eq "draft") $r.Body

# D10. Повторный отзыв -> 409.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/withdraw" -Session $teacherSession
Invoke-Check "D10. Повторный withdraw -> 409" ($r.Status -eq 409) $r.Body

# D11. Копирование курса.
$dupID = ""
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/duplicate" -Session $teacherSession
if ($r.Status -eq 201 -and $r.Json -ne $null) { $dupID = [string]$r.Json.course.id }
$titleOk = $false
if ($r.Status -eq 201) { $titleOk = ([string]$r.Json.course.title).EndsWith("(копия)") }
Invoke-Check "D11. Копия курса -> 201, черновик, title с «(копия)»" `
    ($r.Status -eq 201 -and $dupID -ne "" -and $dupID -ne $courseID -and `
     $r.Json.course.status -eq "draft" -and $titleOk) $r.Body

# D12. Копия глубокая: у неё те же 2 модуля и 3 урока. Счётчики есть только
# в админском списке курсов (repo.Course их не несёт) — смотрим глазами админа.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?limit=100" -Session $adminSession
$dupRow = $null
if ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1) {
    $dupRow = @($r.Json.items) | Where-Object { $_.id -eq $dupID } | Select-Object -First 1
}
$deepOk = ($dupRow -ne $null -and $dupRow.modulesCount -eq 2 -and $dupRow.lessonsCount -eq 3)
Invoke-Check "D12. Копия скопирована глубоко: 2 модуля, 3 урока" $deepOk $r.Body

# D13. Архивация.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/archive" -Session $teacherSession
Invoke-Check "D13. Archive -> 200, status=archived" `
    ($r.Status -eq 200 -and $r.Json.course.status -eq "archived") $r.Body

# D14. Повторная архивация -> 409.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/archive" -Session $teacherSession
Invoke-Check "D14. Повторный archive -> 409" ($r.Status -eq 409) $r.Body

# D15. Возврат из архива.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/unarchive" -Session $teacherSession
Invoke-Check "D15. Unarchive -> 200, status=draft" `
    ($r.Status -eq 200 -and $r.Json.course.status -eq "draft") $r.Body

# D16. Повторный unarchive из draft -> 409.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/unarchive" -Session $teacherSession
Invoke-Check "D16. Повторный unarchive -> 409" ($r.Status -eq 409) $r.Body

# D17. Чужой курс: второй преподаватель -> 403.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/duplicate" -Session $teacher2Session
Invoke-Check "D17. Дублирование чужого курса -> 403" ($r.Status -eq 403) $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- E. Ученики курса (T22) ---" -ForegroundColor Cyan

# E1. Публикация: submit + approve админом.
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/submit" -Session $teacherSession
$submitted = ($r.Status -eq 200)
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/moderate" -Session $adminSession `
    -Body @{ decision = "approve" }
Invoke-Check "E1. Публикация курса (submit + approve) -> published" `
    ($submitted -and $r.Status -eq 200 -and $r.Json.status -eq "published") $r.Body

# E2. Покупка учеником.
$providerPaymentId = ""
$r = Invoke-Api -Method POST -Path "/api/v1/enrollments" -Session $studentSession `
    -Body @{ courseId = $courseID; provider = "mock" }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $providerPaymentId = [string]$r.Json.providerPaymentId }
$r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" `
    -Body @{ providerPaymentId = $providerPaymentId; status = "succeeded" }
Invoke-Check "E2. Покупка курса учеником (mock succeeded)" `
    ($providerPaymentId -ne "" -and $r.Status -eq 200 -and $r.Json.status -eq "processed") $r.Body

# E3. Список учеников курса.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/students" -Session $teacherSession
$row = $null
if ($r.Status -eq 200 -and $r.Json.items -ne $null -and @($r.Json.items).Count -ge 1) {
    $row = @($r.Json.items)[0]
}
Invoke-Check "E3. GET /teacher/courses/{id}/students -> 200, ученик в списке" `
    ($r.Status -eq 200 -and $r.Json.total -eq 1 -and $row -ne $null) $r.Body

# E4. Поля строки ученика.
$fieldsOk = $false
if ($row -ne $null) {
    $fieldsOk = ($row.studentId -eq $studentId -and $row.email -eq $studentEmail -and
                 $row.status -eq "active" -and $row.totalLessons -eq 3 -and
                 $row.completedLessons -eq 0 -and $row.percent -eq 0 -and
                 $row.enrolledAt -ne $null -and $row.lastActivityAt -eq $null)
}
Invoke-Check "E4. Поля строки: email, totalLessons=3, percent=0, lastActivityAt=null" $fieldsOk $r.Body

# E5. Поиск по email.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/students?q=$studentEmail" -Session $teacherSession
Invoke-Check "E5. Поиск ученика по email -> 1" ($r.Status -eq 200 -and $r.Json.total -eq 1) $r.Body

# E6. Поиск без совпадений -> пустой массив, не null.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/students?q=zzz-nope-zzz" -Session $teacherSession
Invoke-Check "E6. Поиск без совпадений -> total=0, items=[]" `
    ($r.Status -eq 200 -and $r.Json.total -eq 0 -and @($r.Json.items).Count -eq 0 -and `
     -not $r.Body.Contains('"items":null')) $r.Body

# E7. Пагинация limit=1.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/students?limit=1" -Session $teacherSession
Invoke-Check "E7. Пагинация limit=1 -> 1 элемент, total=1" `
    ($r.Status -eq 200 -and @($r.Json.items).Count -eq 1 -and $r.Json.total -eq 1) $r.Body

# E8. Ученик проходит урок.
$r = Invoke-Api -Method POST -Path "/api/v1/me/courses/$courseID/lessons/$lessonVideoB/complete" `
    -Session $studentSession
Invoke-Check "E8. Ученик прошёл урок -> passed=true" `
    ($r.Status -eq 200 -and $r.Json.passed -eq $true) $r.Body

# E9. Прогресс виден учителю.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/students" -Session $teacherSession
$row2 = $null
if ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1) { $row2 = @($r.Json.items)[0] }
$progressOk = $false
if ($row2 -ne $null) {
    $progressOk = ($row2.completedLessons -eq 1 -and $row2.percent -eq 33 -and $row2.lastActivityAt -ne $null)
}
Invoke-Check "E9. Прогресс ученика у учителя: 1/3, 33%, активность не null" $progressOk $r.Body

# E10. Статистика курса.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/stats" -Session $teacherSession
$statsOk = $false
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $statsOk = ($r.Json.studentsTotal -eq 1 -and $r.Json.studentsActive -eq 1 -and
                $r.Json.studentsCompleted -eq 0 -and $r.Json.avgPercent -eq 33 -and
                $r.Json.revenueCents -eq 150000 -and $r.Json.modulesTotal -eq 2 -and
                $r.Json.lessonsTotal -eq 3)
}
Invoke-Check "E10. Статистика: 1 ученик, выручка 150000, 3 урока" $statsOk $r.Body

# E11. Чужому преподавателю -> 403.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/students" -Session $teacher2Session
Invoke-Check "E11. Чужому преподавателю -> 403" ($r.Status -eq 403) $r.Body

# E12. Ученику -> 403.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseID/students" -Session $studentSession
Invoke-Check "E12. Ученику -> 403" ($r.Status -eq 403) $r.Body

# E13. Несуществующий курс -> 404.
$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/00000000-0000-0000-0000-0000000000ff/students" `
    -Session $teacherSession
Invoke-Check "E13. Несуществующий курс -> 404" ($r.Status -eq 404) $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- F. Кабинет ученика (T23) ---" -ForegroundColor Cyan

# F1. Прогресс по курсу.
$r = Invoke-Api -Method GET -Path "/api/v1/me/courses/$courseID/progress" -Session $studentSession
$prog = $r.Json
$progOk = $false
if ($r.Status -eq 200 -and $prog -ne $null) {
    $progOk = ($prog.completedLessons -eq 1 -and $prog.totalLessons -eq 3 -and
               $prog.percent -eq 33 -and $prog.completed -eq $false -and
               @($prog.completedLessonIds).Count -eq 1 -and $prog.nextLessonId -ne $null)
}
Invoke-Check "F1. GET /me/courses/{id}/progress: 1/3, 33%, есть nextLessonId" $progOk $r.Body

# F2. Пустой прогресс — массив, а не null.
$r = Invoke-Api -Method GET -Path "/api/v1/me/courses/$dupID/progress" -Session $studentSession
Invoke-Check "F2. Прогресс по курсу без записи -> 403" ($r.Status -eq 403) $r.Body

# F3. Несуществующий курс -> 404.
$r = Invoke-Api -Method GET -Path "/api/v1/me/courses/00000000-0000-0000-0000-0000000000ff/progress" `
    -Session $studentSession
Invoke-Check "F3. Прогресс несуществующего курса -> 404" ($r.Status -eq 404) $r.Body

# F4. Профиль.
$r = Invoke-Api -Method GET -Path "/api/v1/me/profile" -Session $studentSession
$profOk = $false
if ($r.Status -eq 200 -and $r.Json -ne $null) {
    $profOk = ($r.Json.id -eq $studentId -and $r.Json.email -eq $studentEmail -and
               $r.Json.role -eq "student" -and $r.Json.createdAt -ne $null -and
               $r.Json.PSObject.Properties.Name -contains "displayName")
}
Invoke-Check "F4. GET /me/profile -> id, email, role, displayName, createdAt" $profOk $r.Body

# F5. Сохранение имени.
$r = Invoke-Api -Method PATCH -Path "/api/v1/me/profile" -Session $studentSession `
    -Body @{ displayName = "Иван QA" }
Invoke-Check "F5. PATCH /me/profile -> 200, имя сохранено (кириллица)" `
    ($r.Status -eq 200 -and $r.Json.displayName -eq "Иван QA") $r.Body

# F6. Слишком короткое имя -> 400.
$r = Invoke-Api -Method PATCH -Path "/api/v1/me/profile" -Session $studentSession `
    -Body @{ displayName = "A" }
Invoke-Check "F6. Имя из 1 символа -> 400" ($r.Status -eq 400) $r.Body

# F7. Слишком длинное имя -> 400.
$longName = "Я" * 61
$r = Invoke-Api -Method PATCH -Path "/api/v1/me/profile" -Session $studentSession `
    -Body @{ displayName = $longName }
Invoke-Check "F7. Имя из 61 символа -> 400" ($r.Status -eq 400) $r.Body

# F8. История платежей.
$r = Invoke-Api -Method GET -Path "/api/v1/me/payments" -Session $studentSession
$pay = $null
if ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1) { $pay = @($r.Json.items)[0] }
$payOk = $false
if ($pay -ne $null) {
    $payOk = ($pay.courseId -eq $courseID -and $pay.courseTitle -ne "" -and
              $pay.amountCents -eq 150000 -and $pay.status -eq "succeeded" -and
              $pay.processedAt -ne $null)
}
Invoke-Check "F8. GET /me/payments: курс, сумма, статус, processedAt" $payOk $r.Body

# F9. Пагинация платежей.
$r = Invoke-Api -Method GET -Path "/api/v1/me/payments?limit=1" -Session $studentSession
Invoke-Check "F9. Пагинация платежей limit=1" `
    ($r.Status -eq 200 -and @($r.Json.items).Count -eq 1 -and $r.Json.total -ge 1) $r.Body

# F10. Без авторизации -> 401.
$r = Invoke-Api -Method GET -Path "/api/v1/me/payments"
Invoke-Check "F10. /me/payments без авторизации -> 401" ($r.Status -eq 401) $r.Body

# =============================================================================
Write-Host ""
Write-Host "--- G. Супер-админ (T24/T25) ---" -ForegroundColor Cyan

# G1. Список пользователей.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/users" -Session $adminSession
$usersOk = ($r.Status -eq 200 -and $r.Json.total -ge 4 -and @($r.Json.items).Count -ge 4 -and
            -not $r.Body.Contains('"items":null'))
Invoke-Check "G1. GET /admin/users -> 200, конверт items/total/page/limit" $usersOk $r.Body

# G2. Поля строки пользователя.
$u = $null
if ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1) {
    $u = @($r.Json.items) | Where-Object { $_.id -eq $studentId } | Select-Object -First 1
}
$uFields = $false
if ($u -ne $null) {
    $names = $u.PSObject.Properties.Name
    $uFields = (($names -contains "email") -and ($names -contains "role") -and
                ($names -contains "displayName") -and ($names -contains "status") -and
                ($names -contains "createdAt") -and ($names -contains "coursesCount") -and
                ($names -contains "enrollmentsCount"))
}
Invoke-Check "G2. Поля строки: email, role, displayName, status, createdAt, счётчики" $uFields $r.Body

# G3. Поиск по email.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/users?q=$teacherEmail" -Session $adminSession
Invoke-Check "G3. Поиск пользователя по email -> 1" ($r.Status -eq 200 -and $r.Json.total -eq 1) $r.Body

# G4. Фильтр по роли.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/users?role=student" -Session $adminSession
$rolesOk = $true
foreach ($it in @($r.Json.items)) { if ($it.role -ne "student") { $rolesOk = $false } }
Invoke-Check "G4. Фильтр role=student -> только ученики" `
    ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1 -and $rolesOk) $r.Body

# G5. Мусорная роль -> 400.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/users?role=bogus" -Session $adminSession
Invoke-Check "G5. role=bogus -> 400" ($r.Status -eq 400) $r.Body

# G6. Смена роли: ученик -> учитель и обратно.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$studentId/role" -Session $adminSession `
    -Body @{ role = "teacher" }
$toTeacher = ($r.Status -eq 200 -and $r.Json.user.role -eq "teacher")
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$studentId/role" -Session $adminSession `
    -Body @{ role = "student" }
$backToStudent = ($r.Status -eq 200 -and $r.Json.user.role -eq "student")
Invoke-Check "G6. Смена роли ученик↔учитель -> 200 в обе стороны" `
    ($toTeacher -and $backToStudent) $r.Body

# G7. Мусорная роль в теле -> 400.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$studentId/role" -Session $adminSession `
    -Body @{ role = "superuser" }
Invoke-Check "G7. role=superuser -> 400" ($r.Status -eq 400) $r.Body

# G8. Смена собственной роли -> 409.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$adminId/role" -Session $adminSession `
    -Body @{ role = "student" }
Invoke-Check "G8. Смена своей роли -> 409 (защита от запирания)" ($r.Status -eq 409) $r.Body

# G9. Удаление себя -> 409.
$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/users/$adminId" -Session $adminSession
Invoke-Check "G9. Удаление себя -> 409" ($r.Status -eq 409) $r.Body

# G10. Блокировка преподавателя.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$teacherId/status" -Session $adminSession `
    -Body @{ status = "blocked" }
Invoke-Check "G10. Блокировка -> 200, status=blocked" `
    ($r.Status -eq 200 -and $r.Json.user.status -eq "blocked") $r.Body

# G11. Заблокированный не входит.
$blockedSession = New-Session
$r = Invoke-Api -Method POST -Path "/api/v1/auth/login" -Session $blockedSession `
    -Body @{ email = $teacherEmail; password = $password }
Invoke-Check "G11. Вход заблокированного -> 403" ($r.Status -eq 403) $r.Body

# G12. Ранее выданная сессия тоже не работает.
$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $teacherSession
Invoke-Check "G12. Старая сессия заблокированного -> 403" ($r.Status -eq 403) $r.Body

# G13. Разблокировка возвращает доступ.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$teacherId/status" -Session $adminSession `
    -Body @{ status = "active" }
$unblocked = ($r.Status -eq 200 -and $r.Json.user.status -eq "active")
$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $teacherSession
Invoke-Check "G13. Разблокировка -> 200, доступ вернулся" `
    ($unblocked -and $r.Status -eq 200) $r.Body

# G14. Блокировка себя -> 409.
$r = Invoke-Api -Method PATCH -Path "/api/v1/admin/users/$adminId/status" -Session $adminSession `
    -Body @{ status = "blocked" }
Invoke-Check "G14. Блокировка себя -> 409" ($r.Status -eq 409) $r.Body

# G15. Удаление пользователя с платежами -> 409.
$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/users/$studentId" -Session $adminSession
Invoke-Check "G15. Удаление ученика с платежами -> 409 (история сохраняется)" `
    ($r.Status -eq 409) $r.Body

# G16. Сводка платформы.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/stats" -Session $adminSession
$s = $r.Json
$statsShape = $false
if ($r.Status -eq 200 -and $s -ne $null) {
    $statsShape = ($s.users.total -ge 4 -and $s.users.students -ge 1 -and
                   $s.users.teachers -ge 2 -and $s.users.admins -ge 1 -and
                   $s.courses.total -ge 2 -and $s.courses.published -ge 1 -and
                   $s.payments.count -ge 1 -and $s.payments.succeededCents -ge 150000 -and
                   $s.enrollments.total -ge 1 -and $s.reviews.total -ge 0)
}
Invoke-Check "G16. GET /admin/stats -> users/courses/payments/enrollments/reviews" $statsShape $r.Body

# G17. Все курсы.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses" -Session $adminSession
$c = $null
if ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1) {
    $c = @($r.Json.items) | Where-Object { $_.id -eq $courseID } | Select-Object -First 1
}
$courseRowOk = $false
if ($c -ne $null) {
    $names = $c.PSObject.Properties.Name
    $courseRowOk = (($names -contains "teacherEmail") -and ($names -contains "modulesCount") -and
                    ($names -contains "lessonsCount") -and ($names -contains "studentsCount") -and
                    ($names -contains "priceCents"))
}
Invoke-Check "G17. GET /admin/courses: наши курсы со счётчиками и teacherEmail" `
    ($r.Status -eq 200 -and $courseRowOk) $r.Body

# G18. Фильтр по статусу.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?status=published" -Session $adminSession
$pubOk = $true
foreach ($it in @($r.Json.items)) { if ($it.status -ne "published") { $pubOk = $false } }
Invoke-Check "G18. Фильтр status=published -> только опубликованные" `
    ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1 -and $pubOk) $r.Body

# G19. Мусорный статус -> 400.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses?status=bogus" -Session $adminSession
Invoke-Check "G19. status=bogus -> 400" ($r.Status -eq 400) $r.Body

# G20. Очередь модерации не затёрта новым маршрутом.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/review" -Session $adminSession
Invoke-Check "G20. GET /admin/courses/review -> 200 (маршрут не затёрт)" `
    ($r.Status -eq 200 -and $r.Json -ne $null) $r.Body

# G21. Отзыв ученика и модерация отзыва.
$reviewID = ""
$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseID/reviews" -Session $studentSession `
    -Body @{ rating = 5; body = "Отзыв для проверки модерации" }
Invoke-Check "G21. Ученик оставил отзыв -> 201" ($r.Status -eq 201) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/$courseID/reviews" -Session $adminSession
$rev = $null
if ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1) { $rev = @($r.Json.items)[0] }
if ($rev -ne $null) { $reviewID = [string]$rev.id }
Invoke-Check "G22. GET /admin/courses/{id}/reviews: отзыв с email ученика" `
    ($r.Status -eq 200 -and $reviewID -ne "" -and $rev.studentEmail -eq $studentEmail) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/reviews/$reviewID" -Session $adminSession
Invoke-Check "G23. DELETE /admin/reviews/{id} -> 204" ($r.Status -eq 204) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/reviews/$reviewID" -Session $adminSession
Invoke-Check "G24. Повторное удаление отзыва -> 404" ($r.Status -eq 404) $r.Body

# G25. Снятие с публикации.
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/unpublish" -Session $adminSession
Invoke-Check "G25. Unpublish -> 200, status=draft" `
    ($r.Status -eq 200 -and $r.Json.course.status -eq "draft") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/unpublish" -Session $adminSession
Invoke-Check "G26. Повторный unpublish -> 409" ($r.Status -eq 409) $r.Body

# G27. Архивация админом.
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/archive" -Session $adminSession
Invoke-Check "G27. Archive админом -> 200, status=archived" `
    ($r.Status -eq 200 -and $r.Json.course.status -eq "archived") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/archive" -Session $adminSession
Invoke-Check "G28. Повторный archive -> 409" ($r.Status -eq 409) $r.Body

# G29. Все записи на курсы.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/enrollments" -Session $adminSession
$enrOk = $false
if ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1) {
    $e = @($r.Json.items) | Where-Object { $_.courseId -eq $courseID } | Select-Object -First 1
    if ($e -ne $null) {
        $names = $e.PSObject.Properties.Name
        $enrOk = (($names -contains "studentEmail") -and ($names -contains "courseTitle") -and
                  ($names -contains "percent") -and ($names -contains "completedLessons"))
    }
}
Invoke-Check "G29. GET /admin/enrollments: записи с email ученика и прогрессом" `
    ($r.Status -eq 200 -and $enrOk) $r.Body

# G30. Фильтр записей по курсу.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/enrollments?courseId=$courseID" -Session $adminSession
$filterOk = $true
foreach ($it in @($r.Json.items)) { if ($it.courseId -ne $courseID) { $filterOk = $false } }
Invoke-Check "G30. Фильтр записей по courseId" `
    ($r.Status -eq 200 -and @($r.Json.items).Count -ge 1 -and $filterOk) $r.Body

# G31. Преподавателю админ-эндпоинты закрыты.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/stats" -Session $teacherSession
Invoke-Check "G31. Преподавателю /admin/stats -> 403" ($r.Status -eq 403) $r.Body

# G32. Анониму админ-эндпоинты закрыты.
$r = Invoke-Api -Method GET -Path "/api/v1/admin/users"
Invoke-Check "G32. Анониму /admin/users -> 401" ($r.Status -eq 401) $r.Body

# G33. Удаление «свободного» пользователя -> 204.
$freeEmail = "p3-free-$stamp@example.test"
$freeSession = New-Session
$freeId = ""
$r = Invoke-Api -Method POST -Path "/api/v1/auth/register" -Session $freeSession `
    -Body @{ email = $freeEmail; password = $password }
if ($r.Status -eq 201 -and $r.Json -ne $null) { $freeId = [string]$r.Json.id }
$r = Invoke-Api -Method DELETE -Path "/api/v1/admin/users/$freeId" -Session $adminSession
$deleted = ($r.Status -eq 204)
$r = Invoke-Api -Method GET -Path "/api/v1/admin/users?q=$freeEmail" -Session $adminSession
Invoke-Check "G33. Удаление свободного пользователя -> 204, исчез из списка" `
    ($deleted -and $r.Status -eq 200 -and $r.Json.total -eq 0) $r.Body

# =============================================================================
Write-Host ""
$total = $script:Passed + $script:Failed + $script:Skipped
Write-Host "=== qa-p3: $script:Passed успешно, $script:Failed провалено, $script:Skipped пропущено (из $total) ===" `
    -ForegroundColor Cyan

if ($script:Failed -gt 0) {
    Write-Host "ЕСТЬ ПРОВАЛЫ" -ForegroundColor Red
    exit 1
}
Write-Host "Все проверки пройдены" -ForegroundColor Green
exit 0
