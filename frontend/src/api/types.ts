// Доменные типы API-клиента. Имена полей — строго по Task/00-api-contract.md
// и фактическим JSON-формам backend/internal/handler/*.go, repo/models.go.
// Здесь только типы; транспорт и функции — в client.ts.

// ---------- Примитивы и перечисления ----------

/** Роль пользователя. */
export type Role = 'student' | 'teacher' | 'admin';

/** Тип урока. */
export type LessonKind = 'video_text' | 'live' | 'quiz' | 'code';

/** Статус курса. Статуса 'rejected' в backend НЕТ: при отклонении курс возвращается в 'draft'. */
export type CourseStatus = 'draft' | 'review' | 'published' | 'archived';

/** Статус платежа. */
export type PaymentStatus = 'pending' | 'succeeded' | 'failed' | 'refunded';

/** Платёжный провайдер. */
export type Provider = 'mock' | 'stripe' | 'yookassa';

/** Решение по модерации курса. */
export type ModerationDecision = 'approve' | 'reject';

/** Универсальный конверт пагинации. */
export interface Paged<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
}

// ---------- Каталог: фильтры (T65/T66) ----------

/** Категория курса — фиксированный список слагов контракта P8. */
export type CourseCategory = 'programming' | 'design' | 'data' | 'marketing' | 'ai' | 'other';

/** Фильтр цены каталога: бесплатные / платные. */
export type CoursePriceFilter = 'free' | 'paid';

/** Допустимые значения фильтра минимального рейтинга (как строки запроса). */
export type CourseMinRatingFilter = '3.5' | '4' | '4.5';

/** Сортировка каталога (по умолчанию popular). */
export type CourseSort = 'popular' | 'new' | 'rating' | 'price_asc' | 'price_desc';

/** Статистика категории из GET /categories. */
export interface CategoryStat {
  /** Слаг категории из фиксированного списка. */
  slug: string;
  /** Число опубликованных курсов (может быть 0). */
  coursesCount: number;
}

// ---------- Пользователь ----------

export interface User {
  id: string;
  email: string;
  role: Role;
  /** Отдаётся бэкендом после доработки auth (T10); в текущем хендлере поля ещё нет. */
  displayName?: string;
}

// ---------- Курс ----------

/**
 * Карточка курса для каталога и «Моих курсов» (handler.courseItem).
 * Внимание: полей status, teacherName, coverUrl здесь НЕТ — не рассчитывай на них.
 */
export interface CourseItem {
  id: string;
  slug: string;
  title: string;
  description: string;
  priceCents: number;
  currency: string;
  avgRating: number;
  /** Именно ratingsCount, не reviewsCount. */
  ratingsCount: number;
  /** T65: категория курса (слаг фиксированного списка); аддитивное поле. */
  category?: string;
  /**
   * T54: путь к обложке "/uploads/covers/<имя>" (аддитивно с волны P7).
   * null/"" — обложки нет; плитки без обложки рисуются как раньше.
   */
  coverPath?: string | null;
}

/** Полный курс (repo.Course, JSON-теги backend). */
export interface Course {
  id: string;
  teacherId: string;
  slug: string;
  title: string;
  description: string;
  /**
   * T54: путь к обложке "/uploads/covers/<имя>"; отсутствует или null,
   * если обложки нет (omitempty на бэкенде после DELETE → пусто).
   */
  coverPath?: string | null;
  priceCents: number;
  currency: string;
  status: CourseStatus;
  avgRating: number;
  /** Именно ratingsCount, не reviewsCount. */
  ratingsCount: number;
  /** Заполняется при отклонении на модерации (omitempty). */
  moderationComment?: string;
  createdAt: string;
  updatedAt: string;
}

/**
 * Страница курса: getCourse распаковывает конверт { course, modules }
 * и возвращает курс с вложенными модулями.
 */
export interface CourseDetail extends Course {
  modules: Module[];
}

// ---------- Публичный профиль преподавателя (P9, T82) ----------

/**
 * Шапка публичного профиля (GET /teachers/{id}, контракт T81).
 * displayName может быть пустым — UI показывает нейтральное «Преподаватель».
 */
export interface TeacherPublic {
  id: string;
  /** Отображаемое имя; пустая строка, если не задано. */
  displayName: string;
  /** Число published-курсов преподавателя. */
  coursesCount: number;
  /** Активные записи на published-курсы. */
  studentsCount: number;
  /** Средневзвешенный рейтинг по published-курсам; 0 — оценок нет. */
  avgRating: number;
  /** Суммарное число оценок по published-курсам. */
  ratingsCount: number;
}

/**
 * Ответ GET /teachers/{id}: шапка профиля и все published-курсы.
 * Порядок курсов серверный (активные записи desc, затем createdAt desc).
 */
export interface TeacherProfile {
  teacher: TeacherPublic;
  courses: CourseItem[];
}

// ---------- Структура курса ----------

/** Модуль в виде курса (handler.moduleView). */
export interface Module {
  id: string;
  title: string;
  position: number;
  lessons: Lesson[];
}

/**
 * Урок в виде курса (handler.lessonView).
 * moduleId и createdAt есть в полной форме (repo.Lesson), но отсутствуют
 * в публичном view — поэтому опциональны.
 */
export interface Lesson {
  id: string;
  moduleId?: string;
  title: string;
  kind: LessonKind;
  position: number;
  /** Markdown-текст урока (omitempty). */
  bodyMd?: string;
  videoUrl?: string;
  liveUrl?: string;
  /** RFC3339 (omitempty). */
  startsAt?: string;
  /** Квиз: { questions: [...] }. В публичном виде поля correct и explanation может не быть. */
  quiz?: QuizData;
  /** Кодовое задание; в публичном виде приходят только тесты с hidden != true. */
  codeTask?: CodeTask;
  /** Есть только в полной форме (repo.Lesson). */
  createdAt?: string;
}

/**
 * Вопрос квиза. В публичном виде (/courses/{slug}) поле correct отсутствует —
 * ответы не утекают; в запросе создания (createLesson) correct обязателен.
 */
export interface QuizQuestion {
  id: string;
  /** Именно text, не question. */
  text: string;
  options: string[];
  /** Индекс правильного ответа [0, len(options)); в публичном виде отсутствует. */
  correct?: number;
  explanation?: string;
}

/** Обёртка вопросов квиза: именно объект { questions }, не массив. */
export interface QuizData {
  questions: QuizQuestion[];
}

/** Тест кодового задания. */
export interface CodeTest {
  name: string;
  input: string;
  expected: string;
  /** В публичном виде скрытые тесты не приходят (omitempty). */
  hidden?: boolean;
}

/** Кодовое задание урока: объект, не строка. */
export interface CodeTask {
  language: string;
  template?: string;
  tests: CodeTest[];
}

// ---------- Отзывы ----------

/** Отзыв (repo.Review). Поля authorEmail backend НЕ отдаёт. */
export interface Review {
  id: string;
  /** Определить «свой» отзыв можно только по studentId vs User.id. */
  studentId: string;
  courseId: string;
  rating: number;
  /** Текст отзыва называется body, не text (omitempty). */
  body?: string;
  createdAt: string;
  /** T69: ответ преподавателя; пустая строка/отсутствие — ответа нет. */
  teacherReply?: string;
  /** T69: когда ответ оставлен или обновлён; null — ответа нет. */
  teacherReplyAt?: string | null;
}

/** T69: разбивка оценок 5→1 по всем отзывам курса (ключи — строки "1".."5"). */
export type ReviewBreakdown = Record<'1' | '2' | '3' | '4' | '5', number>;

/**
 * T69: конверт GET /courses/{id}/reviews целиком.
 * avgRating/ratingsCount/breakdown считаются по всем отзывам и не зависят
 * от фильтра rating; items/total — по фильтру.
 */
export interface CourseReviewsPage {
  items: Review[];
  total: number;
  page: number;
  limit: number;
  avgRating: number;
  ratingsCount: number;
  breakdown: ReviewBreakdown;
}

// ---------- Обучение ----------

/** Следующий урок для продолжения прохождения. */
export interface NextLesson {
  lessonId: string;
  title: string;
  kind: LessonKind;
}

/** Мой курс с прогрессом (GET /me/courses, конверт распакован клиентом). */
export interface MyCourse {
  /** Карточка курса в форме CourseItem. */
  course: CourseItem;
  percent: number;
  nextLesson: NextLesson | null;
  /**
   * T67/T68: номер сертификата по курсу (аддитивное поле волны P8).
   * Пустая строка или отсутствие — сертификат ещё не выдан.
   */
  certificateNumber?: string;
}

/** Результат завершения урока (POST .../complete). Ключ — passed, не accepted. */
export interface CompleteResult {
  passed: boolean;
  completed: boolean;
  /** Процент по курсу; заполняется при успехе. */
  percent?: number;
  nextLesson?: NextLesson | null;
  /** Текст причины провала (quiz/code). */
  message?: string;
  /** Для quiz при провале: сколько верных из total. */
  correct?: number;
  total?: number;
}

// ---------- Сертификаты ----------

/** Публичный сертификат (GET /certificates/{number}, без авторизации). */
export interface CertificatePublic {
  number: string;
  /** RFC3339. */
  issuedAt: string;
  /** displayName студента; пустая строка, если имя не задано. */
  studentName: string;
  courseTitle: string;
  courseSlug: string;
}

/** Мой сертификат (GET /me/certificates, конверт распакован клиентом). */
export interface MyCertificate {
  number: string;
  /** RFC3339. */
  issuedAt: string;
  courseId: string;
  courseTitle: string;
  courseSlug: string;
}

// ---------- Покупка ----------

/** Результат создания записи на курс (POST /enrollments). */
export interface EnrollmentResult {
  paymentId: string;
  providerPaymentId: string;
  /** Ссылка на оплату; пустая для бесплатных курсов. */
  redirectUrl: string;
  amountCents: number;
  status: PaymentStatus;
  /** true — курс бесплатный и активен сразу. */
  enrollmentActive: boolean;
}

// ---------- Промокоды (P8, T63/T64) ----------

/** Промокод курса (модель PromoCode контракта P8). */
export interface PromoCode {
  id: string;
  courseId: string;
  /** Хранится и сравнивается в верхнем регистре. */
  code: string;
  /** Скидка в процентах, 1..90. */
  discountPct: number;
  /** 0 — без ограничения. */
  maxUses: number;
  usedCount: number;
  /** RFC3339 или null — бессрочный. */
  expiresAt: string | null;
  active: boolean;
  createdAt: string;
}

/** Результат проверки промокода (POST /courses/{id}/promo/validate). */
export interface PromoValidation {
  code: string;
  discountPct: number;
  priceCents: number;
  finalPriceCents: number;
}

// ---------- Админ ----------

/** Приглашение учителя (POST /admin/invites). Токен показывается один раз! */
export interface Invite {
  id: string;
  email: string;
  /** Полный токен — сохранять в UI сразу, повторно он недоступен. */
  token: string;
  tokenPrefix: string;
  expiresAt: string;
}

/** Строка журнала платежей US6 (GET /admin/payments). */
export interface PaymentJournalItem {
  id: string;
  studentId: string;
  /** Email студента. Пустая строка, если в таблице users уже нет такой записи. */
  studentEmail: string;
  /** Отображаемое имя студента; пусто, если не заполнено — тогда UI показывает email. */
  studentName: string;
  courseId: string;
  courseTitle: string;
  amountCents: number;
  platformFeeCents: number;
  teacherAmountCents: number;
  currency: string;
  provider: string;
  providerPaymentId: string;
  status: PaymentStatus;
  createdAt: string;
  /** null — платёж ещё не обработан. */
  processedAt: string | null;
}

/** Итоги журнала платежей за выборку. */
export interface PaymentTotals {
  amountCents: number;
  platformFeeCents: number;
  teacherAmountCents: number;
}

/** Ответ GET /admin/payments (конверт с пагинацией и итогами). */
export interface PaymentJournalPage {
  items: PaymentJournalItem[];
  total: number;
  page: number;
  limit: number;
  totals: PaymentTotals;
}

/** Настройки платформы (GET/PUT /admin/settings). */
export interface AdminSettings {
  /** Комиссия платформы в процентах, 0..90. */
  commissionPct: number;
  /** Пост-P7: включён ли промо-баннер «Первый курс со скидкой» (тогл админки). */
  promoBarEnabled: boolean;
}

/** Публичные настройки интерфейса (GET /settings/public) — без авторизации. */
export interface PublicSettings {
  /** Показывать ли гостевой промо-баннер сверху страниц. */
  promoBarEnabled: boolean;
}

// ---------- Входные формы запросов ----------

/** Вопрос квиза для создания урока: correct обязателен и в диапазоне вариантов. */
export interface QuizQuestionInput {
  /** Если не передать — backend сгенерирует. */
  id?: string;
  text: string;
  options: string[];
  correct: number;
  explanation?: string;
}

/**
 * Тело POST /teacher/courses/{id}/lessons.
 * Поля, не относящиеся к kind, клиент не отправляет (DisallowUnknownFields → 400).
 */
export interface CreateLessonInput {
  moduleId: string;
  title: string;
  kind: LessonKind;
  /** Для kind='video_text'. */
  bodyMd?: string;
  videoUrl?: string;
  /** Для kind='live'. */
  liveUrl?: string;
  /** RFC3339-строка, для kind='live'. */
  startsAt?: string;
  /** Для kind='quiz'. */
  quiz?: { questions: QuizQuestionInput[] };
  /** Для kind='code'. */
  codeTask?: { language: string; template?: string; tests: CodeTest[] };
}

/** Тело POST .../complete: для video/live можно не слать вообще. */
export interface CompleteLessonInput {
  /** Для kind='quiz': { "<questionId>": индексВарианта }. */
  quizAnswers?: Record<string, number>;
  /** Для kind='code'. */
  code?: string;
  /** Необязательно: если пусто, backend берёт язык из задания урока. */
  language?: string;
}
