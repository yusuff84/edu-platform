import { useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { ApiError, login, register } from '../api/client';
import { t } from '../i18n/t';

const loginSchema = z.object({
  email: z.string().min(1, 'Укажите email').email('Некорректный email'),
  password: z.string().min(1, 'Укажите пароль'),
});

const registerSchema = z.object({
  email: z.string().min(1, 'Укажите email').email('Некорректный email'),
  password: z.string().min(8, 'Пароль не короче 8 символов'),
  inviteToken: z.string().optional(),
});

type LoginForm = z.infer<typeof loginSchema>;
type RegisterForm = z.infer<typeof registerSchema>;

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

export function Login(): JSX.Element {
  const navigate = useNavigate();
  const location = useLocation();
  const qc = useQueryClient();
  const [serverError, setServerError] = useState('');
  // Куда вернуться после входа; невалидный/подделанный from → null → уйдём на '/'.
  const fromPath = resolveRedirectFrom(location.state);
  const {
    register: rf,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginForm>({ resolver: zodResolver(loginSchema) });

  const mutation = useMutation({
    mutationFn: (v: LoginForm) => login(v),
    onSuccess: () => {
      setServerError('');
      void qc.invalidateQueries({ queryKey: ['me'] });
      // replace: «назад» после входа не возвращает на форму логина.
      navigate(fromPath ?? '/', { replace: true });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.status === 401) setServerError(t('auth.errLoginFailed'));
      else setServerError(e instanceof Error ? e.message : t('common.error'));
    },
  });

  return (
    <div className="wrap">
      <div className="page">
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
                <input
                  id="login-password"
                  className="input"
                  type="password"
                  autoComplete="current-password"
                  {...rf('password')}
                />
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
            <p>
              {/* Пробрасываем state, чтобы from не потерялся при переходе Login ↔ Register. */}
              <Link to="/register" state={location.state}>{t('auth.noAccount')}</Link>
            </p>
          </div>
          <div className="panel panel-dark">
            <h2>Почему Edu Platform</h2>
            <ul className="checklist">
              <li>Практика с автопроверкой</li>
              <li>Живые разборы</li>
              <li>Доступ навсегда</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}

export function Register(): JSX.Element {
  const navigate = useNavigate();
  const location = useLocation();
  const qc = useQueryClient();
  const [serverError, setServerError] = useState('');
  // Та же валидация from, что и на входе.
  const fromPath = resolveRedirectFrom(location.state);
  const {
    register: rf,
    handleSubmit,
    formState: { errors },
  } = useForm<RegisterForm>({ resolver: zodResolver(registerSchema) });

  const mutation = useMutation({
    mutationFn: (v: RegisterForm) =>
      register({
        email: v.email,
        password: v.password,
        inviteToken: v.inviteToken?.trim() ? v.inviteToken.trim() : undefined,
      }),
    onSuccess: (_res, variables) => {
      setServerError('');
      void qc.invalidateQueries({ queryKey: ['me'] });
      // Приглашённый по invite-токену регистрируется с ролью teacher —
      // логичный старт для него кабинет учителя /teach. Явный from
      // (намерение пользователя) приоритетнее этой эвристики.
      const invited = Boolean(variables.inviteToken?.trim());
      navigate(fromPath ?? (invited ? '/teach' : '/'), { replace: true });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && (e.status === 400 || e.status === 409))
        setServerError(t('auth.errRegisterFailed'));
      else setServerError(e instanceof Error ? e.message : t('common.error'));
    },
  });

  return (
    <div className="wrap">
      <div className="page">
        <div className="two two-even">
          <div className="panel">
            <h1>{t('auth.registerTitle')}</h1>
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
                <input
                  id="register-password"
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  {...rf('password')}
                  aria-describedby="pw-hint"
                />
                <small id="pw-hint">{t('auth.passwordHint')}</small>
                {errors.password && <span className="err">{errors.password.message}</span>}
              </div>
              <div className="field">
                <label htmlFor="register-invite">{t('auth.inviteToken')}</label>
                <input id="register-invite" className="input" type="text" {...rf('inviteToken')} />
              </div>
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
            <p>
              {/* Пробрасываем state, чтобы from не потерялся при переходе Register ↔ Login. */}
              <Link to="/login" state={location.state}>{t('auth.haveAccount')}</Link>
            </p>
          </div>
          <div className="panel panel-sun">
            <h2>Что вы получите</h2>
            <ul className="checklist">
              <li>Бесплатный модуль «Старт»</li>
              <li>Проверка кода</li>
              <li>Эфиры по четвергам</li>
              <li>Сертификат</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
