<#
.SYNOPSIS
    qa-abc.ps1 — сквозной QA-сценарий edu-platform (задача T12).
    Проверяет API против ЗАПУЩЕННОГО стенда, без внешних зависимостей.

.DESCRIPTION
    Сценарий проходит три блока (A/B/C):
      A. Аноним + регистрация: healthz, каталог, регистрация студента,
         вход, повторная регистрация (дубль).
      B. Преподаватель + модерация: админ-вход, invite-токен, регистрация
         учителя, создание курса, валидация submit, модуль, quiz-урок,
         reject/approve, настройки комиссии, 403 для студента на /admin/*.
      C. Покупка + обучение + админка: страница курса, mock-покупка через
         redirectUrl /pay/mock/{id}, вебхук (обработан/дубль), «мои курсы»,
         прохождение квиза (провал и зачёт), отзыв и дубль-409, журнал
         платежей US6, CORS, Яндекс-OAuth без ключей (503), выход и 401.

    Каждый запуск использует уникальные email (qa-<timestamp>@example.test),
    поэтому скрипт можно гонять повторно на одном стенде.

    CSRF: все не-GET запросы шлются с заголовком Origin: http://localhost:3000
    (входит в ALLOWED_ORIGINS по умолчанию) — в прод-режиме без него
    запрос получит 403.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER WebUrl
    URL фронтенда (web). Проверяется мягко: недоступность web не проваливает
    прогон (для API-теста web не обязателен).

.PARAMETER AdminEmail / AdminPassword
    Учётка сид-админа. Должна совпадать с ADMIN_EMAIL/ADMIN_PASSWORD,
    заданными при старте стенда (docker compose up), иначе блок B/C.4
    завершится падением. Альтернатива: -SkipAdmin.

.PARAMETER SkipAdmin
    Пропустить админ-блоки (модерация, invite, журнал). Сценарий покупки
    и обучения при этом пропускается — остается только анонимная часть.

.EXAMPLE
    docker compose up -d db api runner
    pwsh -File qa-abc.ps1 -BaseUrl http://localhost:8082
    docker compose down

.NOTES
    Совместим с PowerShell 5.1 (без grep/tr/редиректоров, $PWD не используется).
    Rate-limit: на частых повторных запусках можно выставить
    RATE_LIMIT_AUTH_PER_MIN=0 в окружении api (лимит — 10/мин на IP).
#>
[CmdletBinding()]
param(
    [string]$BaseUrl = "http://localhost:8082",
    [string]$WebUrl = "http://localhost:3000",
    [string]$AdminEmail = "qa-admin@example.test",
    [string]$AdminPassword = "QaAdmin12345",
    [switch]$SkipAdmin
)

$ErrorActionPreference = "Stop"

# ---- Счётчики результатов ---------------------------------------------------
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
            Write-Host "[FAIL] $Name — $Detail" -ForegroundColor Red
        } else {
            Write-Host "[FAIL] $Name" -ForegroundColor Red
        }
    }
}

function Skip-Check {
    param([string]$Name, [string]$Reason)
    $script:Skipped++
    Write-Host "[SKIP] $Name — $Reason" -ForegroundColor Yellow
}

# ---- HTTP-обёртка -----------------------------------------------------------
# Возвращает объект {Status, Body, Json, Headers}. Ошибки HTTP не бросают
# исключение, а превращаются в Status + Body — так проверяются и 4xx/5xx.
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

    # CSRF: не-GET запросы обязаны нести Origin из ALLOWED_ORIGINS.
    # Явный Origin вызывающего не перезаписываем (для проверки CORS).
    $allHeaders = @{}
    foreach ($k in $Headers.Keys) { $allHeaders[$k] = $Headers[$k] }
    if (-not $allHeaders.ContainsKey("Origin")) {
        $allHeaders["Origin"] = "http://localhost:3000"
    }

    $params = @{
        Method        = $Method
        Uri           = $uri
        Headers       = $allHeaders
        UseBasicParsing = $true
    }
    if ($Session -ne $null) { $params.WebSession = $Session }
    if ($Body -ne $null) {
        $params.Body = ($Body | ConvertTo-Json -Depth 8)
        $params.ContentType = "application/json"
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
            # Тело ошибки: в PS5.1 читаем из потока, в PS7 — из ErrorDetails.
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

# ---- Заголовок --------------------------------------------------------------
$stamp = (Get-Date).ToString("yyyyMMddHHmmss") + "-" + (Get-Random -Maximum 9999)
$studentEmail = "qa-student-$stamp@example.test"
$teacherEmail = "qa-teacher-$stamp@example.test"

Write-Host "=== qa-abc: сквозной QA edu-platform ===" -ForegroundColor Cyan
Write-Host "API: $BaseUrl | Фронт: $WebUrl | Метка времени: $stamp"
Write-Host "Студент: $studentEmail | Преподаватель: $teacherEmail"
Write-Host ""

# ---- Сначала проверяем, что стенд вообще отвечает ---------------------------
$ping = Invoke-Api -Method GET -Path "/healthz"
if ($ping.Status -ne 200) {
    Write-Host "Стенд недоступен: $BaseUrl/healthz -> $($ping.Status) $($ping.Body)" -ForegroundColor Red
    Write-Host "Запустите стенд: docker compose up -d db api runner" -ForegroundColor Yellow
    exit 1
}

# Мягкая проверка фронтенда: для API-прогона web не обязателен.
try {
    $web = Invoke-WebRequest -Uri $WebUrl -UseBasicParsing -TimeoutSec 5
    Invoke-Check "A0. Фронтенд отвечает ($WebUrl)" ([int]$web.StatusCode -eq 200)
} catch {
    Skip-Check "A0. Фронтенд отвечает ($WebUrl)" "web не поднят — для API-прогона допустимо"
}

# =============================================================================
# БЛОК A — доступ и регистрация
# =============================================================================
Write-Host "`n--- Блок A: доступ и регистрация ---" -ForegroundColor Cyan

# A1. healthz на верхнем уровне — JSON {"status":"ok"} (расхождение №5: не текст).
$r = Invoke-Api -Method GET -Path "/healthz"
Invoke-Check "A1. GET /healthz -> 200" ($r.Status -eq 200 -and $r.Json -ne $null -and $r.Json.status -eq "ok") $r.Body

# A2. healthz под /api/v1.
$r = Invoke-Api -Method GET -Path "/api/v1/healthz"
Invoke-Check "A2. GET /api/v1/healthz -> 200" ($r.Status -eq 200) $r.Body

# A3. Каталог анонимно.
$r = Invoke-Api -Method GET -Path "/api/v1/courses"
Invoke-Check "A3. Каталог /api/v1/courses -> 200" ($r.Status -eq 200 -and $r.Json -ne $null) $r.Body

# A4. Регистрация студента (уникальный email) — 201 + роль student.
$studentSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$r = Invoke-Api -Method POST -Path "/api/v1/auth/register" -Session $studentSession `
    -Body @{ email = $studentEmail; password = "password123" }
Invoke-Check "A4. Регистрация студента -> 201, role=student" `
    ($r.Status -eq 201 -and $r.Json -ne $null -and $r.Json.role -eq "student") $r.Body

# A5. Вход теми же кредами — 200.
$r = Invoke-Api -Method POST -Path "/api/v1/auth/login" -Session $studentSession `
    -Body @{ email = $studentEmail; password = "password123" }
Invoke-Check "A5. Вход студента -> 200" ($r.Status -eq 200) $r.Body

# A6. Повторная регистрация того же email — отказ (фактически 400;
# контракт п.1 обещает 409 — зафиксировано в отчёте T12 как расхождение).
$r = Invoke-Api -Method POST -Path "/api/v1/auth/register" `
    -Body @{ email = $studentEmail; password = "password123" }
Invoke-Check "A6. Дубль регистрации отклонён (400)" ($r.Status -eq 400) $r.Body

# A7. Анонимный доступ к /me/courses — 401.
$r = Invoke-Api -Method GET -Path "/api/v1/me/courses"
Invoke-Check "A7. Аноним на /me/courses -> 401" ($r.Status -eq 401) $r.Body

# =============================================================================
# БЛОК B — преподаватель, invite, модерация
# =============================================================================
Write-Host "`n--- Блок B: преподаватель и модерация ---" -ForegroundColor Cyan

$teacherSession = $null
$courseID = $null
$courseSlug = $null
$moduleID = $null
$quizLessonID = $null

if ($SkipAdmin) {
    Skip-Check "B*. Админ-блок (invite, модерация)" "задан -SkipAdmin"
} else {
    # B1. Вход админа (сид создаётся из ADMIN_EMAIL/ADMIN_PASSWORD при старте).
    $adminSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $r = Invoke-Api -Method POST -Path "/api/v1/auth/login" -Session $adminSession `
        -Body @{ email = $AdminEmail; password = $AdminPassword }
    Invoke-Check "B1. Вход админа ($AdminEmail) -> 200, role=admin" `
        ($r.Status -eq 200 -and $r.Json -ne $null -and $r.Json.role -eq "admin") $r.Body
    $adminOk = ($r.Status -eq 200)

    if ($adminOk) {
        # B2. Приглашение учителя — 201, токен виден один раз.
        $r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Session $adminSession `
            -Body @{ email = $teacherEmail }
        $inviteToken = ""
        if ($r.Status -eq 201 -and $r.Json -ne $null) { $inviteToken = [string]$r.Json.token }
        Invoke-Check "B2. POST /admin/invites -> 201 с токеном" ($r.Status -eq 201 -and $inviteToken -ne "") $r.Body

        # B3. Регистрация учителя по invite-токену — роль teacher.
        if ($inviteToken -ne "") {
            $teacherSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
            $r = Invoke-Api -Method POST -Path "/api/v1/auth/register" -Session $teacherSession `
                -Body @{ email = $teacherEmail; password = "password123"; inviteToken = $inviteToken }
            Invoke-Check "B3. Регистрация по invite -> 201, role=teacher" `
                ($r.Status -eq 201 -and $r.Json -ne $null -and $r.Json.role -eq "teacher") $r.Body
        } else {
            Invoke-Check "B3. Регистрация по invite -> 201, role=teacher" $false "нет токена из B2"
        }
    }

    if ($teacherSession -ne $null) {
        # B4. Создание курса — 201, автосlug course-xxxxxxxx, статус draft.
        $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Session $teacherSession `
            -Body @{ title = "QA-курс $stamp"; description = "Сквозной сценарий T12"; priceCents = 2500 }
        if ($r.Status -eq 201 -and $r.Json -ne $null) {
            $courseID = [string]$r.Json.id
            $courseSlug = [string]$r.Json.slug
        }
        Invoke-Check "B4. Создание курса -> 201, draft, slug course-*" `
            ($r.Status -eq 201 -and $courseSlug.StartsWith("course-") -and $r.Json.status -eq "draft") $r.Body

        # B5. Submit пустого курса — 400.
        if ($courseID) {
            $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/submit" -Session $teacherSession -Body @{}
            Invoke-Check "B5. Submit пустого курса -> 400" ($r.Status -eq 400) $r.Body

            # B6. Модуль — 201.
            $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/modules" -Session $teacherSession `
                -Body @{ title = "Основы" }
            if ($r.Status -eq 201) { $moduleID = [string]$r.Json.id }
            Invoke-Check "B6. Создание модуля -> 201" ($r.Status -eq 201 -and $moduleID -ne "") $r.Body
        }

        # B7. Quiz-урок — 201.
        if ($moduleID) {
            $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/lessons" -Session $teacherSession `
                -Body @{
                    moduleId = $moduleID; title = "Квиз"; kind = "quiz"
                    quiz     = @{ questions = @(@{ id = "q1"; text = "Сколько будет 2+2?"; options = @("3", "4"); correct = 0 }) }
                }
            if ($r.Status -eq 201) { $quizLessonID = [string]$r.Json.id }
            Invoke-Check "B7. Quiz-урок -> 201" ($r.Status -eq 201 -and $quizLessonID -ne "") $r.Body
        }

        # B8. Submit с содержимым — 200, status=review.
        $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/submit" -Session $teacherSession -Body @{}
        Invoke-Check "B8. Submit -> 200, status=review" ($r.Status -eq 200 -and $r.Json.status -eq "review") $r.Body

        # B9. Очередь модерации содержит курс.
        $r = Invoke-Api -Method GET -Path "/api/v1/admin/courses/review" -Session $adminSession
        $inQueue = $false
        if ($r.Status -eq 200 -and $r.Json -ne $null -and $r.Json.items -ne $null) {
            foreach ($it in $r.Json.items) { if ([string]$it.id -eq $courseID) { $inQueue = $true } }
        }
        Invoke-Check "B9. Курс в очереди модерации" ($r.Status -eq 200 -and $inQueue) $r.Body

        # B10. Reject без комментария — 400.
        $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/moderate" -Session $adminSession `
            -Body @{ decision = "reject" }
        Invoke-Check "B10. Reject без комментария -> 400" ($r.Status -eq 400) $r.Body

        # B11. Reject с комментарием — 200, draft + moderationComment.
        $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/moderate" -Session $adminSession `
            -Body @{ decision = "reject"; comment = "Проверка T12: мало материалов" }
        Invoke-Check "B11. Reject с комментарием -> 200, draft + moderationComment" `
            ($r.Status -eq 200 -and $r.Json.status -eq "draft" -and ([string]$r.Json.moderationComment -ne "")) $r.Body

        # B12. Повторная подача и approve — published.
        $r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseID/submit" -Session $teacherSession -Body @{}
        $resubmitOk = ($r.Status -eq 200)
        $r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseID/moderate" -Session $adminSession `
            -Body @{ decision = "approve" }
        Invoke-Check "B12. Resubmit + approve -> published" ($resubmitOk -and $r.Status -eq 200 -and $r.Json.status -eq "published") $r.Body

        # B13. Настройки комиссии читаются.
        $r = Invoke-Api -Method GET -Path "/api/v1/admin/settings" -Session $adminSession
        Invoke-Check "B13. GET /admin/settings -> commissionPct" `
            ($r.Status -eq 200 -and $r.Json -ne $null -and $r.Json.commissionPct -ne $null) $r.Body

        # B14. Студенту на /admin/* — 403.
        $r = Invoke-Api -Method GET -Path "/api/v1/admin/payments" -Session $studentSession
        Invoke-Check "B14. Студент на /admin/payments -> 403" ($r.Status -eq 403) $r.Body
    } else {
        foreach ($n in @("B4", "B5", "B6", "B7", "B8", "B9", "B10", "B11", "B12", "B13", "B14")) {
            Skip-Check "$n" "нет учителя/админа (падение B1/B2/B3)"
        }
    }
}

# =============================================================================
# БЛОК C — покупка, обучение, отзывы, админка
# =============================================================================
Write-Host "`n--- Блок C: покупка, обучение, админка ---" -ForegroundColor Cyan

if (-not $courseID -or -not $quizLessonID) {
    Skip-Check "C1..C17" "блок B не прошёл: курс или quiz-урок не созданы"
} else {
    # C1. Страница курса анонимно; правильные ответы квиза не утекают.
    $r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseSlug"
    $noLeak = ($r.Status -eq 200 -and (-not $r.Body.Contains('"correct"')))
    Invoke-Check "C1. Страница курса анонимно, correct не утекает" $noLeak $r.Body

    # C2. Покупка: enroll -> 201, redirectUrl ведёт на /pay/mock/.
    $r = Invoke-Api -Method POST -Path "/api/v1/enrollments" -Session $studentSession `
        -Body @{ courseId = $courseID; provider = "mock" }
    $redirectUrl = ""
    $providerPaymentId = ""
    if ($r.Status -eq 201 -and $r.Json -ne $null) {
        $redirectUrl = [string]$r.Json.redirectUrl
        $providerPaymentId = [string]$r.Json.providerPaymentId
    }
    Invoke-Check "C2. enroll(mock) -> 201, redirectUrl=/pay/mock/*" `
        ($r.Status -eq 201 -and $redirectUrl.StartsWith("/pay/mock/")) $r.Body

    # C3. Страница mock-оплаты открывается (смонтирована T13 на верхнем уровне).
    $r = Invoke-Api -Method GET -Path "" -AbsoluteUri ($BaseUrl + $redirectUrl)
    Invoke-Check "C3. Страница оплаты /pay/mock/{id} -> 200" `
        ($r.Status -eq 200 -and $r.Body.Contains('id="pay"')) $r.Body

    # C4. Вебхук «оплата прошла» — processed (Origin не нужен: подпись/T09).
    $r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" `
        -Body @{ providerPaymentId = $providerPaymentId; status = "succeeded" }
    Invoke-Check "C4. Webhook succeeded -> processed" `
        ($r.Status -eq 200 -and $r.Json.status -eq "processed") $r.Body

    # C5. Дубль вебхука — идемпотентный duplicate.
    $r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" `
        -Body @{ providerPaymentId = $providerPaymentId; status = "succeeded" }
    Invoke-Check "C5. Дубль вебхука -> duplicate" `
        ($r.Status -eq 200 -and $r.Json.status -eq "duplicate") $r.Body

    # C6. «Мои курсы»: курс появился, percent=0, nextLesson — объект.
    $r = Invoke-Api -Method GET -Path "/api/v1/me/courses" -Session $studentSession
    $itemOk = $false
    if ($r.Status -eq 200 -and $r.Json.total -ge 1) {
        $it = $r.Json.items[0]
        $itemOk = ($it.percent -eq 0 -and $it.nextLesson -ne $null -and $it.nextLesson.lessonId -ne $null)
    }
    Invoke-Check "C6. /me/courses: 1 курс, percent=0, nextLesson-объект" $itemOk $r.Body

    # C7. Квиз с неверным ответом — passed=false.
    $r = Invoke-Api -Method POST -Path "/api/v1/me/courses/$courseID/lessons/$quizLessonID/complete" `
        -Session $studentSession -Body @{ quizAnswers = @{ q1 = 1 } }
    Invoke-Check "C7. Квиз (неверно) -> passed=false" `
        ($r.Status -eq 200 -and $r.Json.passed -eq $false) $r.Body

    # C8. Квиз с верным ответом — passed=true, percent=100.
    $r = Invoke-Api -Method POST -Path "/api/v1/me/courses/$courseID/lessons/$quizLessonID/complete" `
        -Session $studentSession -Body @{ quizAnswers = @{ q1 = 0 } }
    Invoke-Check "C8. Квиз (верно) -> passed=true, percent=100" `
        ($r.Status -eq 200 -and $r.Json.passed -eq $true -and $r.Json.percent -eq 100) $r.Body

    # C9. Отзыв после записи — 201.
    $r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseID/reviews" -Session $studentSession `
        -Body @{ rating = 5; body = "Отличный сквозной курс!" }
    Invoke-Check "C9. Отзыв -> 201" ($r.Status -eq 201) $r.Body

    # C10. Повторный отзыв — 409.
    $r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseID/reviews" -Session $studentSession `
        -Body @{ rating = 4; body = "Второй отзыв" }
    Invoke-Check "C10. Повторный отзыв -> 409" ($r.Status -eq 409) $r.Body

    # C11. Список отзывов: avgRating=5.
    $r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseID/reviews"
    Invoke-Check "C11. Список отзывов, avgRating=5" `
        ($r.Status -eq 200 -and $r.Json.avgRating -eq 5) $r.Body

    # C12. Журнал платежей US6: наш платёж, сплит сходится.
    if (-not $SkipAdmin) {
        $r = Invoke-Api -Method GET -Path "/api/v1/admin/payments" -Session $adminSession
        $rowOk = $false
        if ($r.Status -eq 200 -and $r.Json.total -ge 1 -and $r.Json.items.Count -ge 1) {
            $found = $false
            foreach ($row in $r.Json.items) {
                if (([string]$row.id -ne "") -and ($row.platformFeeCents + $row.teacherAmountCents -eq $row.amountCents)) {
                    $found = $true
                }
            }
            $rowOk = $found -and ($r.Json.totals.amountCents -ge $r.Json.total)
        }
        Invoke-Check "C12. GET /admin/payments: есть платёж, сплит fee+teacher=amount" $rowOk $r.Body

        # C13. Фильтр журнала по courseId.
        $r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?courseId=$courseID" -Session $adminSession
        Invoke-Check "C13. Журнал с фильтром courseId -> 200" ($r.Status -eq 200 -and $r.Json.total -ge 1) $r.Body
    } else {
        Skip-Check "C12. Журнал платежей US6" "-SkipAdmin"
        Skip-Check "C13. Фильтр журнала courseId" "-SkipAdmin"
    }

    # C14. CORS: чужой Origin не получает Access-Control-Allow-Origin.
    $r = Invoke-Api -Method GET -Path "/api/v1/courses" -Headers @{ Origin = "https://evil.example" }
    $acaoEvil = ""
    if ($r.Headers -ne $null) { $acaoEvil = [string]$r.Headers["Access-Control-Allow-Origin"] }
    Invoke-Check "C14. Чужой Origin без ACAO-заголовка" ($r.Status -eq 200 -and $acaoEvil -eq "") "ACAO=$acaoEvil"

    # C15. CORS: разрешённый Origin получает ровно свой origin.
    $r = Invoke-Api -Method GET -Path "/api/v1/courses" -Headers @{ Origin = "http://localhost:3000" }
    $acaoGood = ""
    if ($r.Headers -ne $null) { $acaoGood = [string]$r.Headers["Access-Control-Allow-Origin"] }
    Invoke-Check "C15. Разрешённый Origin получает ACAO" ($acaoGood -eq "http://localhost:3000") "ACAO=$acaoGood"

    # C16. Яндекс-OAuth без ключей — 503 (если ключи заданы — 302-редирект).
    $r = Invoke-Api -Method GET -Path "/api/v1/auth/yandex/start"
    Invoke-Check "C16. /auth/yandex/start -> 503 (или 302 при настроенных ключах)" `
        ($r.Status -eq 503 -or $r.Status -eq 302) "status=$($r.Status) $($r.Body)"

    # C17. Выход студента и 401 после него.
    $r = Invoke-Api -Method POST -Path "/api/v1/auth/logout" -Session $studentSession -Body @{}
    $logoutOk = ($r.Status -eq 200 -or $r.Status -eq 204)   # фактически 200 (расхождение №2)
    $r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $studentSession
    Invoke-Check "C17. Logout, затем /auth/me -> 401" ($logoutOk -and $r.Status -eq 401) $r.Body
}

# =============================================================================
# Итог
# =============================================================================
Write-Host ""
Write-Host "=== Итог: $script:Passed пройдено, $script:Failed провалено, $script:Skipped пропущено ===" -ForegroundColor Cyan
if ($script:Failed -gt 0) {
    Write-Host "РАСХОЖДЕНИЕ С ОЖИДАЕМЫМ ПОВЕДЕНИЕМ — см. строки [FAIL] выше." -ForegroundColor Red
    exit 1
}
Write-Host "ВСЕ ПРОВЕРКИ ПРОЙДЕНЫ" -ForegroundColor Green
exit 0
