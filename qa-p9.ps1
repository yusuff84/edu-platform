<#
.SYNOPSIS
    qa-p9.ps1 — сквозной QA-прогон волны P9 (задачи T71–T82).

.DESCRIPTION
    Проверяет на живом стенде:
      A. Вопросы и ответы по урокам (T71): публичный список, права, ответы
         преподавателя, удаление, каскад, 404 для черновика.
      B. Избранное (T73): идемпотентные PUT/DELETE, список, изоляция, 404.
      C. Превью-уроки (T75): публичный просмотр без входа, флаг в программе,
         переключение учителем, 404 для не-preview/чужого урока.
      D. Аналитика учителя (T77): сводка (курсы/ученики/выручка/завершаемость/
         рейтинг/last30d), разбивка по курсам, права.
      E. Похожие курсы (T79): та же категория, без самого курса, limit, 404.
      F. Профиль преподавателя (T81): публичный доступ, счётчики, только
         published-курсы, 404 для ученика.

    Все сущности создаются с уникальным суффиксом (p9-*), прогон можно
    повторять на одном стенде.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER Origin
    Origin для CSRF-заголовков и не-GET запросов (ALLOWED_ORIGINS стенда).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-p9.ps1
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

$suffix = Get-Date -Format "yyyyMMddHHmmss"
$password = "password123"

Write-Host "=== qa-p9: сквозной QA волны P9 (T71-T82) ===" -ForegroundColor Cyan
Write-Host "Стенд: $BaseUrl, origin: $Origin, суффикс: $suffix" -ForegroundColor DarkCyan

# ---- учётки ------------------------------------------------------------------

$admin = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Body @{ email = $AdminEmail; password = $AdminPassword } -Session $admin
Invoke-Check "I1. Вход администратора" ($r.Status -eq 200) $r.Body
if ($r.Status -ne 200) {
    Write-Host "=== qa-p9: $script:Passed PASS, $script:Failed FAIL ===" -ForegroundColor Yellow
    return
}

$teacherEmail = "p9-teacher-$suffix@example.test"
$studentEmail = "p9-student-$suffix@example.test"
$student2Email = "p9-student2-$suffix@example.test"

$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Body @{ email = $teacherEmail } -Session $admin
Invoke-Check "I2. Приглашение учителя создано" ($r.Status -eq 201 -and $r.Json.token) $r.Body
$inviteToken = $r.Json.token

$teacher = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $teacherEmail; password = $password; inviteToken = $inviteToken } -Session $teacher
Invoke-Check "I3. Учитель зарегистрирован" ($r.Status -eq 201 -and $r.Json.role -eq "teacher") $r.Body

$student = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $studentEmail; password = $password } -Session $student
Invoke-Check "I4. Первый ученик зарегистрирован" ($r.Status -eq 201) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $student
$studentId = $r.Json.id

$student2 = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $student2Email; password = $password } -Session $student2
Invoke-Check "I5. Второй ученик зарегистрирован" ($r.Status -eq 201) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/auth/me" -Session $teacher
$teacherId = $r.Json.id
Invoke-Check "I6. id преподавателя получен" ($teacherId -ne $null) $r.Body

# ---- курсы -------------------------------------------------------------------

# PA: платный design-курс, два урока (один — превью).
$slugPA = "p9-paid-$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{
    title = "P9 paid $suffix"; description = "QA P9 paid course."; slug = $slugPA
    priceCents = 100000; category = "design"
} -Session $teacher
Invoke-Check "S1. Платный курс создан (design)" ($r.Status -eq 201 -and $r.Json.id -and $r.Json.category -eq "design") $r.Body
$coursePA = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$coursePA/modules" -Body @{ title = "Module PA" } -Session $teacher
$modulePA = $r.Json.id
Invoke-Check "S2. Модуль платного курса создан" ($r.Status -eq 201 -and $modulePA) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$coursePA/lessons" -Body @{
    moduleId = $modulePA; title = "PA lesson preview"; kind = "video_text"
    videoUrl = "https://example.test/p9-preview.mp4"; bodyMd = "Preview body."
    isPreview = $true
} -Session $teacher
Invoke-Check "S3. Урок-превью создан с isPreview=true" ($r.Status -eq 201 -and $r.Json.id -and $r.Json.isPreview -eq $true) $r.Body
$lessonPA1 = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$coursePA/lessons" -Body @{
    moduleId = $modulePA; title = "PA lesson closed"; kind = "video_text"
    videoUrl = "https://example.test/p9-closed.mp4"; bodyMd = "Closed body."
} -Session $teacher
Invoke-Check "S4. Обычный урок создан без превью" ($r.Status -eq 201 -and $r.Json.id -and $r.Json.isPreview -eq $false) $r.Body
$lessonPA2 = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$coursePA/submit" -Session $teacher
Invoke-Check "S5. Платный курс отправлен на модерацию" ($r.Status -eq 200) $r.Body
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$coursePA/moderate" -Body @{ decision = "approve" } -Session $admin
Invoke-Check "S6. Платный курс опубликован" ($r.Status -eq 200 -and $r.Json.status -eq "published") $r.Body

# PB: бесплатный design-курс — для Q&A, похожих, аналитики.
$slugPB = "p9-free-$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{
    title = "P9 free $suffix"; description = "QA P9 free course."; slug = $slugPB
    priceCents = 0; category = "design"
} -Session $teacher
Invoke-Check "S7. Бесплатный курс создан (design)" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$coursePB = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$coursePB/modules" -Body @{ title = "Module PB" } -Session $teacher
$modulePB = $r.Json.id
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$coursePB/lessons" -Body @{
    moduleId = $modulePB; title = "PB lesson one"; kind = "video_text"
    videoUrl = "https://example.test/p9-free.mp4"; bodyMd = "Free body."
} -Session $teacher
$lessonPB = $r.Json.id
Invoke-Check "S8. Модуль и урок бесплатного курса созданы" ($modulePB -and $lessonPB) $r.Body
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$coursePB/submit" -Session $teacher
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$coursePB/moderate" -Body @{ decision = "approve" } -Session $admin
Invoke-Check "S9. Бесплатный курс опубликован" ($r.Status -eq 200 -and $r.Json.status -eq "published") $r.Body

# PD: черновик — не должен светиться публично.
$slugPD = "p9-draft-$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{
    title = "P9 draft $suffix"; description = "QA P9 draft course."; slug = $slugPD
    priceCents = 50000; category = "design"
} -Session $teacher
$coursePD = $r.Json.id
Invoke-Check "S10. Черновик создан" ($r.Status -eq 201 -and $coursePD) $r.Body

# ---- записи ------------------------------------------------------------------

$r = Invoke-Auth -Method POST -Path "/api/v1/enrollments" -Body @{ courseId = $coursePA; provider = "mock" } -Session $student
Invoke-Check "S11. Начата покупка платного курса" ($r.Status -eq 201 -and $r.Json.providerPaymentId) $r.Body
$paidPayment = $r.Json
$r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" -Body @{
    providerPaymentId = $paidPayment.providerPaymentId; status = "succeeded"
} -Session $student
Invoke-Check "S12. Платёж подтверждён" ($r.Status -eq 200 -or $r.Status -eq 204) $r.Body

$r = Invoke-Auth -Method POST -Path "/api/v1/enrollments" -Body @{ courseId = $coursePB; provider = "mock" } -Session $student2
Invoke-Check "S13. Бесплатная запись активна сразу" ($r.Status -eq 201 -and $r.Json.enrollmentActive -eq $true) $r.Body

# ============================================================================
# A. Вопросы и ответы (T71)
# ============================================================================
Write-Host ""
Write-Host "=== A. Вопросы и ответы (T71) ===" -ForegroundColor Cyan

$anon = New-Session

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePB/questions" -Session $anon
Invoke-Check "A1. Публичный список пуст (200, total 0)" ($r.Status -eq 200 -and $r.Json.total -eq 0 -and @($r.Json.items).Count -eq 0) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions" -Body @{ lessonId = $lessonPB; body = "Question from not enrolled user." } -Session $student
Invoke-Check "A2. Вопрос без записи -> 403" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions" -Body @{ lessonId = $lessonPB; body = "short" } -Session $student2
Invoke-Check "A3. Слишком короткий вопрос -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions" -Body @{ lessonId = $lessonPA1; body = "Lesson from another course must be rejected." } -Session $student2
Invoke-Check "A4. Урок не из курса -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions" -Body @{
    lessonId = $lessonPB; body = "How does the free lesson work? I want to understand the details."
} -Session $student2
Invoke-Check "A5. Записанный ученик задал вопрос (201)" ($r.Status -eq 201 -and $r.Json.question.id -and $r.Json.question.lessonId -eq $lessonPB -and @($r.Json.question.answers).Count -eq 0) $r.Body
$question1 = $r.Json.question.id

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions" -Body @{
    lessonId = $lessonPB; body = "Second question: is there a certificate after completion?"
} -Session $student2
$question2 = $r.Json.question.id
Invoke-Check "A6. Второй вопрос создан" ($r.Status -eq 201 -and $question2) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePB/questions?lessonId=$lessonPB&page=1&limit=10" -Session $anon
Invoke-Check "A7. Публичный список: total 2, новые первыми" ($r.Status -eq 200 -and $r.Json.total -eq 2 -and @($r.Json.items).Count -eq 2 -and @($r.Json.items)[0].id -eq $question2) $r.Body
Invoke-Check "A8. В вопросе есть lessonTitle и studentName" ($r.Json.items[0].lessonTitle -eq "PB lesson one" -and $r.Json.items[0].studentName -ne $null) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePB/questions?lessonId=$lessonPA1" -Session $anon
Invoke-Check "A9. Фильтр по чужому уроку -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions/$question1/answers" -Body @{ body = "Answered by a student who is not enrolled." } -Session $student
Invoke-Check "A10. Ответ без записи -> 403" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions/$question1/answers" -Body @{ body = "x" } -Session $teacher
Invoke-Check "A11. Слишком короткий ответ -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions/$question1/answers" -Body @{
    body = "The first lesson is free to preview, and the rest opens after enrollment."
} -Session $teacher
Invoke-Check "A12. Преподаватель ответил (201, isTeacher)" ($r.Status -eq 201 -and $r.Json.answer.isTeacher -eq $true -and $r.Json.answer.authorRole -eq "teacher") $r.Body
$answer1 = $r.Json.answer.id

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions/$question1/answers" -Body @{
    body = "I am also taking this course, thanks for the explanation!"
} -Session $student2
Invoke-Check "A13. Ученик может ответить (isTeacher=false)" ($r.Status -eq 201 -and $r.Json.answer.isTeacher -eq $false) $r.Body
$answer2 = $r.Json.answer.id

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/questions/00000000-0000-0000-0000-000000000000/answers" -Body @{ body = "No such question." } -Session $teacher
Invoke-Check "A14. Ответ на несуществующий вопрос -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePB/questions?lessonId=$lessonPB" -Session $anon
$q1 = @($r.Json.items | Where-Object { $_.id -eq $question1 })[0]
Invoke-Check "A15. Ответы в списке, старые первыми" ($q1 -and @($q1.answers).Count -eq 2 -and @($q1.answers)[0].id -eq $answer1) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$coursePB/questions/$question1/answers/$answer2" -Session $student
Invoke-Check "A16. Удаление чужого ответа -> 403" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$coursePB/questions/$question1/answers/$answer2" -Session $student2
Invoke-Check "A17. Автор удалил свой ответ -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$coursePB/questions/$question1/answers/$answer2" -Session $student2
Invoke-Check "A18. Повторное удаление ответа -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$coursePB/questions/$question1" -Session $student
Invoke-Check "A19. Удаление чужого вопроса -> 403" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$coursePB/questions/$question1" -Session $student2
Invoke-Check "A20. Автор удалил вопрос (каскад ответов) -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePB/questions?lessonId=$lessonPB" -Session $anon
Invoke-Check "A21. После удаления остался один вопрос" ($r.Status -eq 200 -and $r.Json.total -eq 1) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/courses/$coursePB/questions/$question2" -Session $teacher
Invoke-Check "A22. Владелец курса удалил вопрос -> 204" ($r.Status -eq 204) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePD/questions" -Session $anon
Invoke-Check "A23. Вопросы черновика публично -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePB/questions?lessonId=00000000-0000-0000-0000-000000000000" -Session $anon
Invoke-Check "A24. Несуществующий lessonId -> 404" ($r.Status -eq 404) $r.Body

# ============================================================================
# B. Избранное (T73)
# ============================================================================
Write-Host ""
Write-Host "=== B. Избранное (T73) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method PUT -Path "/api/v1/me/favorites/$coursePA" -Session $anon
Invoke-Check "B1. Без входа -> 401" ($r.Status -eq 401) $r.Body

$r = Invoke-Api -Method PUT -Path "/api/v1/me/favorites/$coursePA" -Session $student
Invoke-Check "B2. Добавление в избранное -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method PUT -Path "/api/v1/me/favorites/$coursePA" -Session $student
Invoke-Check "B3. Повторное добавление идемпотентно" ($r.Status -eq 204) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/me/favorites" -Session $student
$fav = @($r.Json.items | Where-Object { $_.id -eq $coursePA })[0]
Invoke-Check "B4. Список содержит курс (CourseItem)" ($r.Status -eq 200 -and $r.Json.total -ge 1 -and $fav -and $fav.slug -eq $slugPA) $r.Body

$r = Invoke-Api -Method PUT -Path "/api/v1/me/favorites/$coursePD" -Session $student
Invoke-Check "B5. Черновик в избранное -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/me/favorites" -Session $student2
$hasFav = @($r.Json.items | Where-Object { $_.id -eq $coursePA }).Count -gt 0
Invoke-Check "B6. Чужое избранное не видно" ($r.Status -eq 200 -and -not $hasFav) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/me/favorites/$coursePA" -Session $student
Invoke-Check "B7. Удаление из избранного -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/me/favorites" -Session $student
$hasFav = @($r.Json.items | Where-Object { $_.id -eq $coursePA }).Count -gt 0
Invoke-Check "B8. После удаления курса нет" ($r.Status -eq 200 -and -not $hasFav) $r.Body
$r = Invoke-Api -Method DELETE -Path "/api/v1/me/favorites/$coursePA" -Session $student
Invoke-Check "B9. Повторное удаление идемпотентно" ($r.Status -eq 204) $r.Body

# ============================================================================
# C. Превью-уроки (T75)
# ============================================================================
Write-Host ""
Write-Host "=== C. Превью-уроки (T75) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPA" -Session $anon
$pubLessons = @()
foreach ($m in @($r.Json.modules)) { $pubLessons += @($m.lessons) }
$pubL1 = @($pubLessons | Where-Object { $_.id -eq $lessonPA1 })[0]
$pubL2 = @($pubLessons | Where-Object { $_.id -eq $lessonPA2 })[0]
Invoke-Check "C1. Публичная программа содержит isPreview (true/false)" ($r.Status -eq 200 -and $pubL1.isPreview -eq $true -and $pubL2.isPreview -eq $false) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPA/preview/$lessonPA1" -Session $anon
Invoke-Check "C2. Превью без входа: 200, курс и урок" ($r.Status -eq 200 -and $r.Json.course.slug -eq $slugPA -and $r.Json.lesson.id -eq $lessonPA1 -and $r.Json.lesson.videoUrl) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPA/preview/$lessonPA2" -Session $anon
Invoke-Check "C3. Не-preview урок -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPB/preview/$lessonPA1" -Session $anon
Invoke-Check "C4. Чужой курс в пути -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPD/preview/$lessonPA1" -Session $anon
Invoke-Check "C5. Превью черновика -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$coursePA/lessons/$lessonPA1" -Body @{ isPreview = $false } -Session $teacher
$patched = $r.Json
if ($patched -and $patched.lesson) { $patched = $patched.lesson }
Invoke-Check "C6. Учитель снял флаг isPreview" ($r.Status -eq 200 -and $patched.isPreview -eq $false) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugPA/preview/$lessonPA1" -Session $anon
Invoke-Check "C7. После снятия флага превью -> 404" ($r.Status -eq 404) $r.Body
$r = Invoke-Api -Method PATCH -Path "/api/v1/teacher/courses/$coursePA/lessons/$lessonPA1" -Body @{ isPreview = $true } -Session $teacher
$patched = $r.Json
if ($patched -and $patched.lesson) { $patched = $patched.lesson }
Invoke-Check "C8. Учитель вернул флаг isPreview" ($r.Status -eq 200 -and $patched.isPreview -eq $true) $r.Body

# ============================================================================
# D. Аналитика учителя (T77)
# ============================================================================
Write-Host ""
Write-Host "=== D. Аналитика учителя (T77) ===" -ForegroundColor Cyan

# Завершаем бесплатный курс и оставляем отзыв — для завершаемости и рейтинга.
$r = Invoke-Api -Method POST -Path "/api/v1/me/courses/$coursePB/lessons/$lessonPB/complete" -Body @{ } -Session $student2
Invoke-Check "D1. Ученик завершил бесплатный урок (100%)" ($r.Status -eq 200 -and $r.Json.percent -eq 100) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$coursePB/reviews" -Body @{ rating = 5; body = "P9 QA review for stats." } -Session $student2
Invoke-Check "D2. Отзыв для рейтинга создан" ($r.Status -eq 201) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/teacher/stats" -Session $teacher
Invoke-Check "D3. Сводка доступна учителю (200)" ($r.Status -eq 200) $r.Body
$stats = $r.Json
Invoke-Check "D4. Курсы: total 3, published 2, draft 1" ($stats.courses.total -eq 3 -and $stats.courses.published -eq 2 -and $stats.courses.draft -eq 1) $r.Body
Invoke-Check "D5. Ученики: 2 активные записи, 2 уникальных" ($stats.students.total -eq 2 -and $stats.students.unique -eq 2) $r.Body
Invoke-Check "D6. Выручка: paidTotal 100000, fee+teacher = paid" ($stats.revenue.paidTotalCents -eq 100000 -and ($stats.revenue.platformFeeCents + $stats.revenue.teacherCents) -eq 100000) $r.Body
Invoke-Check "D7. Завершаемость: 1 завершён, средний прогресс 50" ($stats.completion.completed -eq 1 -and [math]::Abs([double]$stats.completion.avgPercent - 50) -lt 0.01) $r.Body
Invoke-Check "D8. Рейтинг: 5.0, одна оценка" ($stats.ratings.avgRating -eq 5 -and $stats.ratings.ratingsCount -eq 1) $r.Body
Invoke-Check "D9. last30d: 2 записи и 100000 выручки" ($stats.last30d.enrollments -eq 2 -and $stats.last30d.revenueCents -eq 100000) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/teacher/stats/courses?limit=50" -Session $teacher
Invoke-Check "D10. Разбивка по курсам: 3 строки" ($r.Status -eq 200 -and $r.Json.total -eq 3 -and @($r.Json.items).Count -eq 3) $r.Body
$rowPA = @($r.Json.items | Where-Object { $_.courseId -eq $coursePA })[0]
$rowPB = @($r.Json.items | Where-Object { $_.courseId -eq $coursePB })[0]
Invoke-Check "D11. Платный курс: 1 ученик, выручка 100000" ($rowPA -and $rowPA.students -eq 1 -and $rowPA.revenueCents -eq 100000 -and $rowPA.status -eq "published") $r.Body
Invoke-Check "D12. Бесплатный курс: 1 ученик, 100% прогресс, рейтинг 5" ($rowPB -and $rowPB.students -eq 1 -and $rowPB.revenueCents -eq 0 -and [math]::Abs([double]$rowPB.avgPercent - 100) -lt 0.01 -and $rowPB.avgRating -eq 5) $r.Body
Invoke-Check "D13. Первым идёт курс с выручкой" (@($r.Json.items)[0].courseId -eq $coursePA) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/teacher/stats/courses?limit=1" -Session $teacher
Invoke-Check "D14. limit=1 ограничивает выборку" ($r.Status -eq 200 -and @($r.Json.items).Count -eq 1 -and $r.Json.total -eq 3) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/teacher/stats" -Session $student
Invoke-Check "D15. Ученику аналитика -> 403" ($r.Status -eq 403) $r.Body

# ============================================================================
# E. Похожие курсы (T79)
# ============================================================================
Write-Host ""
Write-Host "=== E. Похожие курсы (T79) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePA/similar?limit=4" -Session $anon
$ids = @($r.Json.items | ForEach-Object { $_.id })
$sameCat = $true
foreach ($c in @($r.Json.items)) { if ($c.category -ne "design") { $sameCat = $false } }
Invoke-Check "E1. Похожие: только design, есть бесплатный курс" ($r.Status -eq 200 -and $sameCat -and ($ids -contains $coursePB)) $r.Body
Invoke-Check "E2. Сам курс и черновик не попадают" ($r.Status -eq 200 -and -not ($ids -contains $coursePA) -and -not ($ids -contains $coursePD)) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePA/similar?limit=1" -Session $anon
Invoke-Check "E3. limit=1 возвращает один курс" ($r.Status -eq 200 -and @($r.Json.items).Count -eq 1) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/00000000-0000-0000-0000-000000000000/similar" -Session $anon
Invoke-Check "E4. Несуществующий курс -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$coursePD/similar" -Session $anon
Invoke-Check "E5. Черновик как источник -> 404" ($r.Status -eq 404) $r.Body

# ============================================================================
# F. Профиль преподавателя (T81)
# ============================================================================
Write-Host ""
Write-Host "=== F. Профиль преподавателя (T81) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method GET -Path "/api/v1/teachers/$teacherId" -Session $anon
Invoke-Check "F1. Публичный профиль без входа (200)" ($r.Status -eq 200 -and $r.Json.teacher.id -eq $teacherId) $r.Body
Invoke-Check "F2. Курсов 2, учеников 2, рейтинг 5/1" ($r.Json.teacher.coursesCount -eq 2 -and $r.Json.teacher.studentsCount -eq 2 -and $r.Json.teacher.avgRating -eq 5 -and $r.Json.teacher.ratingsCount -eq 1) $r.Body
$profileCourses = @($r.Json.courses | ForEach-Object { $_.id })
Invoke-Check "F3. В профиле только published-курсы" ($profileCourses -contains $coursePA -and $profileCourses -contains $coursePB -and -not ($profileCourses -contains $coursePD)) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/teachers/00000000-0000-0000-0000-000000000000" -Session $anon
Invoke-Check "F4. Несуществующий преподаватель -> 404" ($r.Status -eq 404) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/teachers/$studentId" -Session $anon
Invoke-Check "F5. Профиль ученика -> 404" ($r.Status -eq 404) $r.Body

# ---- итог --------------------------------------------------------------------
Write-Host ""
$color = "Green"
if ($script:Failed -gt 0) { $color = "Red" }
Write-Host "=== qa-p9: $script:Passed PASS, $script:Failed FAIL, $script:Skipped SKIP ===" -ForegroundColor $color
if ($script:Failed -gt 0) { exit 1 }
