import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
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
  password: z.string().min(8, 'Пароль должен быть не короче 8 символов'),
  inviteToken: z.string().optional(),
});

type LoginForm = z.infer<typeof loginSchema>;
type RegisterForm = z.infer<typeof registerSchema>;

export function Login(): JSX.Element {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [serverError, setServerError] = useState('');
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
      navigate('/');
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
              <Link to="/register">{t('auth.noAccount')}</Link>
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
  const qc = useQueryClient();
  const [serverError, setServerError] = useState('');
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
    onSuccess: () => {
      setServerError('');
      void qc.invalidateQueries({ queryKey: ['me'] });
      navigate('/');
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
              <Link to="/login">{t('auth.haveAccount')}</Link>
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
