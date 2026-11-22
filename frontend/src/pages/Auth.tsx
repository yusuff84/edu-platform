import { useState } from 'react';
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { EMAIL_RE, firstError, makeResolver, textLength } from '../app/formResolver';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, login, register } from '../api/client';
// Те же опции query ['me'], что у RouteGuards/Layout: один ключ — один запрос,
// TanStack Query дедуплицирует. Файл не импортируется обратно сюда (нет цикла),
// а shell.css уже подключён через router.tsx — новых стилей не появляется.
import { meQueryOptions } from '../components/RouteGuards';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import { setSessionUser } from '../app/sessionCache';

// Логин не требует полноценного e-mail: демо- и QA-аккаунты (admin@local,
// teacher@local) заводятся с локальным доменом, а проверку учётных данных
// всё равно делает сервер. Достаточно непустой строки с «@» — иначе
// браузерная валидация блокировала вход в документированные демо-учётки.
type LoginForm = { email: string; password: string };
type RegisterForm = { email: string; password: string; inviteToken?: string };

const loginResolver = makeResolver<LoginForm, LoginForm>((v) => {
  const email = v.email.trim();
  const errors = {
    email: firstError([email.length < 1, 'Укажите email'], [!email.includes('@'), 'Некорректный email']),
    password: firstError([v.password.length < 1, 'Укажите пароль']),
  };
  return errors.email || errors.password ? { errors } : { values: { email, password: v.password } };
});

const registerResolver = makeResolver<RegisterForm, RegisterForm>((v) => {
  const errors = {
    email: firstError([v.email.length < 1, 'Укажите email'], [!EMAIL_RE.test(v.email), 'Некорректный email']),
    password: firstError(
      [textLength(v.password) < 8, 'Пароль не короче 8 символов'],
      [new TextEncoder().encode(v.password).length > 72, 'Пароль слишком длинный. Сократите его.'],
    ),
  };
  return errors.email || errors.password
    ? { errors }
    : { values: { email: v.email, password: v.password, ...(v.inviteToken !== undefined ? { inviteToken: v.inviteToken } : {}) } };
});

/**
 * Внутренний путь SPA: начинается ровно с одного «/», второй символ — не «/»
 * и не «\» (иначе браузер может расценить значение как протокол-относительный
 * или абсолютный URL: «//evil.example», «/\evil.example» — это open redirect).
 */
const INTERNAL_PATH_RE = /^\/(?!\/)(?!\\)/;

/**
 * Достаёт и валидирует state.from, который кладут гварды RequireAuth/RequireRole
 * (см. RouteGuards.tsx) перед редиректом на /login.
 *
 * Формат from — контракт между гвардом и этой страницей; гвард сейчас кладёт
 * строку-путь (location.pathname), но на случай смены формата на объект
 * Location (pathname + search + hash) принимаем и его.
 *
 * Защита от open redirect:
 * 1) путь обязан начинаться с одного «/» (отсекает «https://evil.example»,
 *    «//evil.example», «/\evil.example»);
 * 2) страховочный рубеж — URL разбирается внутри текущего origin
 *    (новый URL с базой location.origin не должен сменить origin).
 */
function resolveRedirectFrom(state: unknown): string | null {
  if (typeof state !== 'object' || state === null) return null;
  const from = (state as { from?: unknown }).from;

  let path: string | null = null;
  if (typeof from === 'string') {
    path = from;
  } else if (typeof from === 'object' && from !== null) {
    // Объект Location от react-router.
    const loc = from as { pathname?: unknown; search?: unknown; hash?: unknown };
    if (typeof loc.pathname !== 'string') return null;
    path =
      loc.pathname +
      (typeof loc.search === 'string' ? loc.search : '') +
      (typeof loc.hash === 'string' ? loc.hash : '');
  }

  if (!path || !INTERNAL_PATH_RE.test(path)) return null;
  try {
    const url = new URL(path, window.location.origin);
    if (url.origin !== window.location.origin) return null;
  } catch {
    return null;
  }
  return path;
}

/** Кнопка «показать/скрыть пароль» внутри поля (на телефоне пароль легко опечатать). */
function PasswordToggle({ shown, onToggle }: { shown: boolean; onToggle: () => void }): JSX.Element {
  return (
    <button
      type="button"
      className="pw-toggle"
      onClick={onToggle}
      aria-pressed={shown}
      aria-label={shown ? 'Скрыть пароль' : 'Показать пароль'}
    >
      {shown ? 'Скрыть' : 'Показать'}
    </button>
  );
}

export function Login(): JSX.Element {
  useSeo('Вход — skillex.space', 'Войдите в skillex.space, чтобы продолжить обучение.');
  const navigate = useNavigate();
  const location = useLocation();
  const qc = useQueryClient();
  const [serverError, setServerError] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  // Куда вернуться после входа; невалидный/подделанный from → null → уйдём на '/'.
  const fromPath = resolveRedirectFrom(location.state);
  // Уже вошедший не должен видеть форму входа. Используем ОБЩИЕ опции
  // query ['me'] (meQueryOptions из RouteGuards) — тот же ключ, что у гвардов
  // и Layout, поэтому TanStack Query дедуплицирует и второго запроса нет.
  const meQuery = useQuery(meQueryOptions);
  const {
    register: rf,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginForm>({ resolver: loginResolver });

  const mutation = useMutation({
    mutationFn: (v: LoginForm) => login(v),
    onSuccess: async (user) => {
      setServerError('');
      await setSessionUser(qc, user);
      // replace: «назад» после входа не возвращает на форму логина.
      navigate(fromPath ?? '/', { replace: true });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 401) setServerError(t('auth.errLoginFailed'));
      else setServerError(e instanceof Error ? e.message : t('common.error'));
    },
  });

  // Сессия уже есть — уводим со страницы входа: если гвард запомнил, откуда
  // пришли (state.from), возвращаем туда (та же безопасная валидация from,
  // что после отправки формы — логику T15 не дублируем), иначе — в кабинет.
  if (meQuery.data && !mutation.isPending && !mutation.isSuccess) return <Navigate to={fromPath ?? '/me'} replace />;

  return (
    <div className="wrap">
      <main className="page">
        <div className="two two-even">
          <div className="panel">
            <h1>{t('auth.loginTitle')}</h1>
            <form className="form-grid" onSubmit={handleSubmit((v) => mutation.mutate(v))} noValidate>
              <div className="field">
                <label htmlFor="login-email">{t('auth.email')}</label>
                <input
                  id="login-email"
                  className="input"
                  type="email"
                  autoComplete="email"
                  {...rf('email')}
                />
                {errors.email && <span className="err">{errors.email.message}</span>}
              </div>
              <div className="field">
                <label htmlFor="login-password">{t('auth.password')}</label>
                <div className="pw-wrap">
                  <input
                    id="login-password"
                    className="input"
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="current-password"
                    {...rf('password')}
                  />
                  <PasswordToggle shown={showPassword} onToggle={() => setShowPassword((v) => !v)} />
                </div>
                {errors.password && <span className="err">{errors.password.message}</span>}
              </div>
              <div>
                <button className="btn btn-primary" type="submit" disabled={mutation.isPending}>
                  {t('auth.submitLogin')}
                </button>
              </div>
            </form>
            {serverError && (
              <p role="alert" className="error">
                {serverError}
              </p>
            )}
            {/* Пробрасываем state, чтобы from не потерялся при переходе Login ↔ Register. */}
            <div className="auth-links">
              <Link to="/forgot-password">Забыли пароль?</Link>
              <span>
                Нет аккаунта? <Link to="/register" state={location.state}>Зарегистрироваться</Link>
              </span>
            </div>
          </div>
          <div className="panel panel-dark">
            <h2>Почему skillex.space</h2>
            <ul className="checklist">
              <li>Практика с автопроверкой</li>
              <li>Видеоуроки и конспекты</li>
              <li>Учитесь в своём темпе</li>
            </ul>
          </div>
        </div>
      </main>
    </div>
  );
}

export function Register(): JSX.Element {
  useSeo('Регистрация — skillex.space', 'Создайте аккаунт skillex.space и начните учиться.');
  const navigate = useNavigate();
  const location = useLocation();
  const qc = useQueryClient();
  const [searchParams] = useSearchParams();
  const [serverError, setServerError] = useState('');
  // Та же валидация from, что и на входе.
  const fromPath = resolveRedirectFrom(location.state);
  // Приглашение учителя (US3): ссылка вида /register?inviteToken=... — админ
  // копирует её, преподаватель переходит. Токен берём из query: раньше он
  // читался только из поля формы, ссылка открывала обычную регистрацию
  // без роли teacher и приглашение не потреблялось.
  const inviteFromUrl = searchParams.get('inviteToken')?.trim() ?? '';
  const [showPassword, setShowPassword] = useState(false);
  // Уже зарегистрированный (сессия есть) — уводим со страницы регистрации.
  // Общие опции query ['me']: дедупликация по ключу, второго запроса нет.
  const meQuery = useQuery(meQueryOptions);
  const {
    register: rf,
    handleSubmit,
    formState: { errors },
  } = useForm<RegisterForm>({
    resolver: registerResolver,
    // Токен из ссылки сразу в поле формы: пользователь видит, что он приглашён,
    // поле остаётся редактируемым, а при отправке значение уходит в register().
    defaultValues: { inviteToken: inviteFromUrl },
  });

  const mutation = useMutation({
    mutationFn: (v: RegisterForm) =>
      register({
        email: v.email,
        password: v.password,
        inviteToken: v.inviteToken?.trim() ? v.inviteToken.trim() : undefined,
      }),
    onSuccess: async (user, variables) => {
      setServerError('');
      await setSessionUser(qc, user);
      // Приглашённый по invite-токену регистрируется с ролью teacher —
      // логичный старт для него кабинет учителя /teach. Явный from
      // (намерение пользователя) приоритетнее этой эвристики.
      const invited = Boolean(variables.inviteToken?.trim());
      navigate(fromPath ?? (invited ? '/teach' : '/'), { replace: true });
    },
    onError: (e: unknown, variables: RegisterForm) => {
      // Отправлен ли токен — решает форма (поле предзаполнено из query).
      const invited = Boolean(variables.inviteToken?.trim());
      if (e instanceof ApiError && (e.status === 400 || e.status === 409)) {
        // Ошибки приглашения приходят из envelope понятным русским текстом
        // («Срок действия приглашения истёк», «Приглашение недействительно
        // или уже использовано», «Приглашение выдано на другой email») —
        // показываем его, а не общий «email уже занят».
        if (invited && e.message) setServerError(e.message);
        else setServerError(t('auth.errRegisterFailed'));
      } else setServerError(e instanceof Error ? e.message : t('common.error'));
    },
  });

  // Сессия уже есть — со страницы регистрации уводим так же, как со входа:
  // в state.from (если гвард его положил), иначе — в кабинет.
  if (meQuery.data && !mutation.isPending && !mutation.isSuccess) return <Navigate to={fromPath ?? '/me'} replace />;

  return (
    <div className="wrap">
      <main className="page">
        <div className="two two-even">
          <div className="panel">
            <h1>{t('auth.registerTitle')}</h1>
            {inviteFromUrl && (
              // Понятное состояние «вы приглашены»: ключа словаря под такую
              // фразу нет (см. отчёт T17) — собираем из существующих.
              <p className="muted" role="status">
                {t('admin.inviteCreated')} — {t('cabinet.teacherTitle')}
              </p>
            )}
            <form className="form-grid" onSubmit={handleSubmit((v) => mutation.mutate(v))} noValidate>
              <div className="field">
                <label htmlFor="register-email">{t('auth.email')}</label>
                <input
                  id="register-email"
                  className="input"
                  type="email"
                  autoComplete="email"
                  {...rf('email')}
                />
                {errors.email && <span className="err">{errors.email.message}</span>}
              </div>
              <div className="field">
                <label htmlFor="register-password">{t('auth.password')}</label>
                <div className="pw-wrap">
                  <input
                    id="register-password"
                    className="input"
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="new-password"
                    {...rf('password')}
                    aria-describedby="pw-hint"
                  />
                  <PasswordToggle shown={showPassword} onToggle={() => setShowPassword((v) => !v)} />
                </div>
                <small id="pw-hint">{t('auth.passwordHint')}</small>
                {errors.password && <span className="err">{errors.password.message}</span>}
              </div>
              {/* Код приглашения нужен только преподавателям: ученику поле не
                  мешает — оно свёрнуто, если код не пришёл в ссылке. */}
              <details className="invite-details" open={Boolean(inviteFromUrl)}>
                <summary>У меня есть код приглашения преподавателя</summary>
                <div className="field">
                  <label htmlFor="register-invite">{t('auth.inviteToken')}</label>
                  <input id="register-invite" className="input" type="text" autoComplete="off" {...rf('inviteToken')} />
                </div>
              </details>
              <div>
                <button className="btn btn-primary" type="submit" disabled={mutation.isPending}>
                  {t('auth.submitRegister')}
                </button>
              </div>
            </form>
            {serverError && (
              <p role="alert" className="error">
                {serverError}
              </p>
            )}
            {/* Пробрасываем state, чтобы from не потерялся при переходе Register ↔ Login. */}
            <div className="auth-links">
              <span>
                Уже есть аккаунт? <Link to="/login" state={location.state}>Войти</Link>
              </span>
            </div>
          </div>
          <div className="panel panel-sun">
            <h2>Что вы получите</h2>
            <ul className="checklist">
              <li>Бесплатные курсы</li>
              <li>Тесты с автопроверкой</li>
              <li>Вопросы преподавателю под уроком</li>
              <li>Сертификат о прохождении</li>
            </ul>
          </div>
        </div>
      </main>
    </div>
  );
}
