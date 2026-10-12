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
    <main>
      <h1>{t('auth.loginTitle')}</h1>
      <form onSubmit={handleSubmit((v) => mutation.mutate(v))} noValidate>
        <label>
          {t('auth.email')}
          <input type="email" autoComplete="email" {...rf('email')} />
          {errors.email && <span className="error">{errors.email.message}</span>}
        </label>
        <label>
          {t('auth.password')}
          <input type="password" autoComplete="current-password" {...rf('password')} />
          {errors.password && <span className="error">{errors.password.message}</span>}
        </label>
        <button type="submit" disabled={mutation.isPending}>
          {t('auth.submitLogin')}
        </button>
      </form>
      {serverError && (
        <p role="alert" className="error">
          {serverError}
        </p>
      )}
      <p>
        <Link to="/register">{t('auth.noAccount')}</Link>
      </p>
    </main>
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
    <main>
      <h1>{t('auth.registerTitle')}</h1>
      <form onSubmit={handleSubmit((v) => mutation.mutate(v))} noValidate>
        <label>
          {t('auth.email')}
          <input type="email" autoComplete="email" {...rf('email')} />
          {errors.email && <span className="error">{errors.email.message}</span>}
        </label>
        <label>
          {t('auth.password')}
          <input
            type="password"
            autoComplete="new-password"
            {...rf('password')}
            aria-describedby="pw-hint"
          />
          <small id="pw-hint">{t('auth.passwordHint')}</small>
          {errors.password && <span className="error">{errors.password.message}</span>}
        </label>
        <label>
          {t('auth.inviteToken')}
          <input type="text" {...rf('inviteToken')} />
        </label>
        <button type="submit" disabled={mutation.isPending}>
          {t('auth.submitRegister')}
        </button>
      </form>
      {serverError && (
        <p role="alert" className="error">
          {serverError}
        </p>
      )}
      <p>
        <Link to="/login">{t('auth.haveAccount')}</Link>
      </p>
    </main>
  );
}
