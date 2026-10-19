<#
.SYNOPSIS
    qa-p8.ps1 — сквозной QA-прогон волны P8 (задачи T63–T70).

.DESCRIPTION
    Проверяет на живом стенде:
      A. Промокоды (T63): создание/дубликат/валидация, покупка со скидкой,
         идемпотентность usedCount, поля промокода в журнале платежей.
      B. Категории и фильтры каталога (T65): GET /categories, фильтры
         category/price/minRating, сортировки, 400 на неизвестные значения.
      C. Сертификаты (T67): автовыдача при 100%, /me/certificates,
         certificateNumber в /me/courses, публичная проверка без входа, 404.
      D. Отзывы (T69): ответ преподавателя (создание/замена/удаление), права,
         фильтр по оценке и breakdown.

    Все сущности создаются с уникальным суффиксом (p8-*), прогон можно
    повторять на одном стенде.

.PARAMETER BaseUrl
    Базовый URL API стенда. По умолчанию http://localhost:8082.

.PARAMETER AdminEmail / AdminPassword
    Админ стенда (ADMIN_EMAIL/ADMIN_PASSWORD при docker compose up).

.PARAMETER Origin
    Origin для CSRF-заголовков и не-GET запросов (ALLOWED_ORIGINS стенда).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\qa-p8.ps1
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
$future = (Get-Date).ToUniversalTime().AddDays(30).ToString("yyyy-MM-ddTHH:mm:ssZ")
$past = (Get-Date).ToUniversalTime().AddDays(-1).ToString("yyyy-MM-ddTHH:mm:ssZ")

Write-Host "=== qa-p8: сквозной QA волны P8 (T63-T70) ===" -ForegroundColor Cyan
Write-Host "Стенд: $BaseUrl, origin: $Origin, суффикс: $suffix" -ForegroundColor DarkCyan

# ---- учётки ------------------------------------------------------------------

$admin = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/login" -Body @{ email = $AdminEmail; password = $AdminPassword } -Session $admin
Invoke-Check "I1. Вход администратора" ($r.Status -eq 200) $r.Body
if ($r.Status -ne 200) {
    Write-Host "=== qa-p8: $script:Passed PASS, $script:Failed FAIL ===" -ForegroundColor Yellow
    return
}

$teacherEmail = "p8-teacher-$suffix@example.test"
$studentEmail = "p8-student-$suffix@example.test"
$student2Email = "p8-student2-$suffix@example.test"

$r = Invoke-Api -Method POST -Path "/api/v1/admin/invites" -Body @{ email = $teacherEmail } -Session $admin
Invoke-Check "I2. Приглашение учителя создано" ($r.Status -eq 201 -and $r.Json.token) $r.Body
$inviteToken = $r.Json.token

$teacher = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $teacherEmail; password = $password; inviteToken = $inviteToken } -Session $teacher
Invoke-Check "I3. Учитель зарегистрирован" ($r.Status -eq 201 -and $r.Json.role -eq "teacher") $r.Body

$student = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $studentEmail; password = $password } -Session $student
Invoke-Check "I4. Ученик зарегистрирован" ($r.Status -eq 201 -and $r.Json.role -eq "student") $r.Body

$student2 = New-Session
$r = Invoke-Auth -Method POST -Path "/api/v1/auth/register" -Body @{ email = $student2Email; password = $password } -Session $student2
Invoke-Check "I5. Второй ученик зарегистрирован" ($r.Status -eq 201) $r.Body

# ============================================================================
# A. Промокоды (T63)
# ============================================================================
Write-Host ""
Write-Host "=== A. Промокоды (T63) ===" -ForegroundColor Cyan

$slugA = "p8-promo-$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{
    title = "P8 promo $suffix"; description = "QA P8 promo course."; slug = $slugA
    priceCents = 100000; category = "design"
} -Session $teacher
Invoke-Check "A1. Платный курс создан с категорией (201)" ($r.Status -eq 201 -and $r.Json.id -and $r.Json.category -eq "design") $r.Body
$courseA = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/modules" -Body @{ title = "Module A" } -Session $teacher
$moduleA = $r.Json.id
Invoke-Check "A2. Модуль создан" ($r.Status -eq 201 -and $moduleA) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/lessons" -Body @{
    moduleId = $moduleA; title = "Lesson A1"; kind = "video_text"
    videoUrl = "https://example.test/p8.mp4"; bodyMd = "P8 promo lesson."
} -Session $teacher
Invoke-Check "A3. Урок создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/submit" -Session $teacher
Invoke-Check "A4. Курс отправлен на модерацию" ($r.Status -eq 200) $r.Body
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseA/moderate" -Body @{ decision = "approve" } -Session $admin
Invoke-Check "A5. Курс опубликован" ($r.Status -eq 200 -and $r.Json.status -eq "published") $r.Body

# создание промокодов
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/promo-codes" -Body @{
    code = "sale25"; discountPct = 25; maxUses = 0; expiresAt = $future
} -Session $teacher
Invoke-Check "A6. Промокод создан, код в верхнем регистре" ($r.Status -eq 201 -and $r.Json.promoCode.code -eq "SALE25") $r.Body
$promoSale = $r.Json.promoCode.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/promo-codes" -Body @{
    code = "SALE25"; discountPct = 10
} -Session $teacher
Invoke-Check "A7. Дубликат кода -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/promo-codes" -Body @{
    code = "EXPIRED1"; discountPct = 10; expiresAt = $past
} -Session $teacher
Invoke-Check "A8. Срок в прошлом -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/promo-codes" -Body @{
    code = "BAD%CODE"; discountPct = 10
} -Session $teacher
Invoke-Check "A9. Некорректный код -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseA/promo-codes" -Body @{
    code = "LIMIT1"; discountPct = 10; maxUses = 1
} -Session $teacher
Invoke-Check "A10. Промокод с лимитом создан" ($r.Status -eq 201 -and $r.Json.promoCode.maxUses -eq 1) $r.Body
$promoLimit = $r.Json.promoCode.id

# валидация
$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/promo/validate" -Body @{ code = "sale25" } -Session $student
Invoke-Check "A11. Валидация: скидка 25% и итоговая цена" ($r.Status -eq 200 -and $r.Json.discountPct -eq 25 -and $r.Json.priceCents -eq 100000 -and $r.Json.finalPriceCents -eq 75000) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/promo/validate" -Body @{ code = "NOPE" } -Session $student
Invoke-Check "A12. Неизвестный код -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/promo/validate" -Body @{ code = "" } -Session $student
Invoke-Check "A13. Пустой код -> 400" ($r.Status -eq 400) $r.Body

# покупка со скидкой
$r = Invoke-Auth -Method POST -Path "/api/v1/enrollments" -Body @{ courseId = $courseA; provider = "mock"; promoCode = "sale25" } -Session $student
Invoke-Check "A14. Покупка со скидкой: amountCents = 75000" ($r.Status -eq 201 -and $r.Json.amountCents -eq 75000 -and $r.Json.paymentId) $r.Body
$pay = $r.Json

$r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" -Body @{ providerPaymentId = $pay.providerPaymentId; status = "succeeded" } -Session $student
Invoke-Check "A15. Вебхук succeeded принят" ($r.Status -eq 200 -or $r.Status -eq 204) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" -Body @{ providerPaymentId = $pay.providerPaymentId; status = "succeeded" } -Session $student
Invoke-Check "A16. Повторный вебхук идемпотентен" ($r.Status -eq 200 -or $r.Status -eq 204) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/teacher/courses/$courseA/promo-codes" -Session $teacher
$sale = @($r.Json.items | Where-Object { $_.code -eq "SALE25" })[0]
Invoke-Check "A17. usedCount = 1 ровно один раз" ($r.Status -eq 200 -and $sale.usedCount -eq 1) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/admin/payments?courseId=$courseA&limit=10" -Session $admin
$payItem = $null
if ($r.Json -and $r.Json.items) { $payItem = @($r.Json.items | Where-Object { $_.promoCode -eq "SALE25" })[0] }
Invoke-Check "A18. Журнал: promoCode и discountCents" ($r.Status -eq 200 -and $payItem -and $payItem.discountCents -eq 25000) $r.Body

# лимит использований
$r = Invoke-Auth -Method POST -Path "/api/v1/enrollments" -Body @{ courseId = $courseA; provider = "mock"; promoCode = "LIMIT1" } -Session $student2
Invoke-Check "A19. Второй ученик купил по LIMIT1 (90000)" ($r.Status -eq 201 -and $r.Json.amountCents -eq 90000) $r.Body
$pay2 = $r.Json
$r = Invoke-Api -Method POST -Path "/api/v1/payments/webhook/mock" -Body @{ providerPaymentId = $pay2.providerPaymentId; status = "succeeded" } -Session $student2
Invoke-Check "A20. Вебхук второго платежа" ($r.Status -eq 200 -or $r.Status -eq 204) $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/promo/validate" -Body @{ code = "LIMIT1" } -Session $student
Invoke-Check "A21. Лимит исчерпан -> 400" ($r.Status -eq 400) $r.Body

# деактивация
$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseA/promo-codes/$promoSale" -Session $teacher
Invoke-Check "A22. Деактивация -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseA/promo-codes/$promoSale" -Session $teacher
Invoke-Check "A23. Повторная деактивация -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/promo/validate" -Body @{ code = "SALE25" } -Session $student
Invoke-Check "A24. Отключённый код -> 400" ($r.Status -eq 400) $r.Body
$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseA/promo/validate" -Body @{ code = "SALE25" } -Session $admin
$null = $r

# ============================================================================
# B. Категории и фильтры каталога (T65)
# ============================================================================
Write-Host ""
Write-Host "=== B. Категории и фильтры каталога (T65) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method GET -Path "/api/v1/categories"
$slugs = @($r.Json.items | ForEach-Object { $_.slug })
$design = @($r.Json.items | Where-Object { $_.slug -eq "design" })[0]
Invoke-Check "B1. GET /categories: шесть слагов в порядке" ($r.Status -eq 200 -and ($slugs -join ",") -eq "programming,design,data,marketing,ai,other") $r.Body
Invoke-Check "B2. Счётчик design >= 1" ($design.coursesCount -ge 1) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?category=design&limit=100"
$allDesign = $true
foreach ($c in @($r.Json.items)) { if ($c.category -ne "design") { $allDesign = $false } }
$hasA = @($r.Json.items | Where-Object { $_.id -eq $courseA }).Count -eq 1
Invoke-Check "B3. Фильтр category=design: только design и наш курс" ($r.Status -eq 200 -and $allDesign -and $hasA) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?category=unknown"
Invoke-Check "B4. Неизвестная категория -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?price=paid&sort=price_asc&limit=100"
$asc = $true; $prev = -1; $allPaid = $true
foreach ($c in @($r.Json.items)) {
    if ($c.priceCents -le 0) { $allPaid = $false }
    if ($c.priceCents -lt $prev) { $asc = $false }
    $prev = $c.priceCents
}
Invoke-Check "B5. price=paid + sort=price_asc" ($r.Status -eq 200 -and $allPaid -and $asc) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?price=weird"
Invoke-Check "B6. Недопустимый price -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?sort=rating&limit=100"
$nonInc = $true; $prev = 999
foreach ($c in @($r.Json.items)) { if ($c.avgRating -gt $prev) { $nonInc = $false }; $prev = $c.avgRating }
Invoke-Check "B7. sort=rating: рейтинг не возрастает" ($r.Status -eq 200 -and $nonInc) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?minRating=4&limit=100"
$onlyRated = $true
foreach ($c in @($r.Json.items)) { if ($c.ratingsCount -le 0 -or $c.avgRating -lt 4) { $onlyRated = $false } }
Invoke-Check "B8. minRating=4: только курсы с рейтингом >= 4" ($r.Status -eq 200 -and $onlyRated) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?minRating=3.6"
Invoke-Check "B9. Недопустимый minRating -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?sort=bad"
Invoke-Check "B10. Недопустимый sort -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$slugA"
Invoke-Check "B11. Публичная карточка содержит category=design" ($r.Status -eq 200 -and $r.Json.course.category -eq "design") $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses?q=$suffix&sort=price_desc&limit=100"
$firstA = $false
if (@($r.Json.items).Count -ge 1) { $firstA = (@($r.Json.items)[0].id -eq $courseA) }
Invoke-Check "B12. q + sort=price_desc: платный курс первым" ($r.Status -eq 200 -and $r.Json.total -ge 1 -and $firstA) $r.Body

# ============================================================================
# C. Сертификаты (T67)
# ============================================================================
Write-Host ""
Write-Host "=== C. Сертификаты (T67) ===" -ForegroundColor Cyan

$slugC = "p8-cert-$suffix"
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses" -Body @{
    title = "P8 cert $suffix"; description = "QA P8 certificate course."; slug = $slugC
    priceCents = 0; category = "data"
} -Session $teacher
Invoke-Check "C1. Бесплатный курс создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$courseC = $r.Json.id

$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseC/modules" -Body @{ title = "Module C" } -Session $teacher
$moduleC = $r.Json.id
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseC/lessons" -Body @{
    moduleId = $moduleC; title = "Lesson C1"; kind = "video_text"
    videoUrl = "https://example.test/c1.mp4"; bodyMd = "Certificate lesson."
} -Session $teacher
$lessonC = $r.Json.id
Invoke-Check "C2. Модуль и урок созданы" ($moduleC -and $lessonC) $r.Body
$r = Invoke-Api -Method POST -Path "/api/v1/teacher/courses/$courseC/submit" -Session $teacher
$r = Invoke-Api -Method POST -Path "/api/v1/admin/courses/$courseC/moderate" -Body @{ decision = "approve" } -Session $admin
Invoke-Check "C3. Курс опубликован" ($r.Status -eq 200 -and $r.Json.status -eq "published") $r.Body

$r = Invoke-Auth -Method POST -Path "/api/v1/enrollments" -Body @{ courseId = $courseC; provider = "mock" } -Session $student
Invoke-Check "C4. Бесплатная запись активна сразу" ($r.Status -eq 201 -and $r.Json.enrollmentActive -eq $true) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/me/courses" -Session $student
$itemC = @($r.Json.items | Where-Object { $_.course.id -eq $courseC })[0]
Invoke-Check "C5. До завершения certificateNumber пуст" ($itemC -and $itemC.certificateNumber -eq "") $r.Body

$r = Invoke-Api -Method POST -Path "/api/v1/me/courses/$courseC/lessons/$lessonC/complete" -Body @{ } -Session $student
Invoke-Check "C6. Урок завершён, прогресс 100%" ($r.Status -eq 200 -and $r.Json.percent -eq 100) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/me/courses" -Session $student
$itemC = @($r.Json.items | Where-Object { $_.course.id -eq $courseC })[0]
$certNumber = $itemC.certificateNumber
Invoke-Check "C7. certificateNumber появился (EDU-...)" ($certNumber -like "EDU-*") $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/me/certificates" -Session $student
$myCert = @($r.Json.items | Where-Object { $_.number -eq $certNumber })[0]
Invoke-Check "C8. /me/certificates содержит сертификат" ($r.Status -eq 200 -and $myCert -and $myCert.courseId -eq $courseC -and $myCert.courseSlug -eq $slugC) $r.Body
$countBefore = @($r.Json.items | Where-Object { $_.courseId -eq $courseC }).Count

$r = Invoke-Api -Method POST -Path "/api/v1/me/courses/$courseC/lessons/$lessonC/complete" -Body @{ } -Session $student
Invoke-Check "C9. Повторное завершение не ломает прогресс" ($r.Status -eq 200) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/me/certificates" -Session $student
$countAfter = @($r.Json.items | Where-Object { $_.courseId -eq $courseC }).Count
Invoke-Check "C10. Сертификат не дублируется" ($countAfter -eq $countBefore -and $countAfter -eq 1) $r.Body

$anon = New-Session
$r = Invoke-Api -Method GET -Path "/api/v1/certificates/$certNumber" -Session $anon
$hasEmail = $false
if ($r.Json) { $hasEmail = ($r.Json.PSObject.Properties.Name -contains "email") }
Invoke-Check "C11. Публичная проверка без входа (200, без email)" ($r.Status -eq 200 -and $r.Json.number -eq $certNumber -and $r.Json.courseSlug -eq $slugC -and -not $hasEmail) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/certificates/EDU-2026-NOTFOUND" -Session $anon
Invoke-Check "C12. Неизвестный номер -> 404" ($r.Status -eq 404) $r.Body

# ============================================================================
# D. Отзывы (T69)
# ============================================================================
Write-Host ""
Write-Host "=== D. Отзывы: ответы и разбивка (T69) ===" -ForegroundColor Cyan

$r = Invoke-Api -Method POST -Path "/api/v1/courses/$courseC/reviews" -Body @{ rating = 5; body = "Great course, thanks!" } -Session $student
Invoke-Check "D1. Отзыв создан" ($r.Status -eq 201 -and $r.Json.id) $r.Body
$reviewId = $r.Json.id

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseC/reviews"
$bd = $r.Json.breakdown
Invoke-Check "D2. breakdown: ключи 1..5, 5 = 1" ($r.Status -eq 200 -and $bd.'5' -eq 1 -and $bd.'4' -eq 0 -and $bd.'1' -eq 0) $r.Body
Invoke-Check "D3. avgRating/ratingsCount по всем отзывам" ($r.Json.avgRating -eq 5 -and $r.Json.ratingsCount -eq 1) $r.Body

$r = Invoke-Api -Method PUT -Path "/api/v1/teacher/courses/$courseC/reviews/$reviewId/reply" -Body @{ body = "Thank you for the feedback!" } -Session $teacher
Invoke-Check "D4. Ответ преподавателя создан" ($r.Status -eq 200 -and $r.Json.review.teacherReply -eq "Thank you for the feedback!" -and $r.Json.review.teacherReplyAt) $r.Body

$r = Invoke-Api -Method PUT -Path "/api/v1/teacher/courses/$courseC/reviews/$reviewId/reply" -Body @{ body = "Updated reply text." } -Session $teacher
Invoke-Check "D5. Повторный PUT заменяет ответ" ($r.Status -eq 200 -and $r.Json.review.teacherReply -eq "Updated reply text.") $r.Body

$r = Invoke-Api -Method PUT -Path "/api/v1/teacher/courses/$courseC/reviews/$reviewId/reply" -Body @{ body = "x" } -Session $teacher
Invoke-Check "D6. Слишком короткий ответ -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method PUT -Path "/api/v1/teacher/courses/$courseC/reviews/$reviewId/reply" -Body @{ body = "Hacked reply text." } -Session $student
Invoke-Check "D7. Чужой (ученик) -> 403" ($r.Status -eq 403) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseC/reviews?rating=5"
Invoke-Check "D8. Фильтр rating=5: total 1, ответ виден" ($r.Status -eq 200 -and $r.Json.total -eq 1 -and @($r.Json.items)[0].teacherReply -eq "Updated reply text.") $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseC/reviews?rating=4"
Invoke-Check "D9. rating=4: items пусто, статистика прежняя" ($r.Status -eq 200 -and $r.Json.total -eq 0 -and $r.Json.avgRating -eq 5 -and $r.Json.breakdown.'5' -eq 1) $r.Body

$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseC/reviews?rating=9"
Invoke-Check "D10. rating вне 1..5 -> 400" ($r.Status -eq 400) $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseC/reviews/$reviewId/reply" -Session $teacher
Invoke-Check "D11. Удаление ответа -> 204" ($r.Status -eq 204) $r.Body
$r = Invoke-Api -Method GET -Path "/api/v1/courses/$courseC/reviews?rating=5"
Invoke-Check "D12. После удаления teacherReply пуст" ($r.Status -eq 200 -and @($r.Json.items)[0].teacherReply -eq "") $r.Body

$r = Invoke-Api -Method DELETE -Path "/api/v1/teacher/courses/$courseC/reviews/$reviewId/reply" -Session $teacher
Invoke-Check "D13. Повторное удаление -> 204" ($r.Status -eq 204) $r.Body

# ---- итог --------------------------------------------------------------------
Write-Host ""
$color = "Green"
if ($script:Failed -gt 0) { $color = "Red" }
Write-Host "=== qa-p8: $script:Passed PASS, $script:Failed FAIL, $script:Skipped SKIP ===" -ForegroundColor $color
if ($script:Failed -gt 0) { exit 1 }
