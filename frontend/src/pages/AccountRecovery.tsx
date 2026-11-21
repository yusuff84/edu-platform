import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { forgotPassword, resetPassword, verifyEmail } from '../api/accountSecurity';
import { setSessionUser } from '../app/sessionCache';
import { useSeo } from '../app/useSeo';
import '../styles/accountSecurity.css';

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Не удалось выполнить запрос. Повторите позже.';
}

// Read once, then remove the token from the address bar/history. The fragment
// is never sent to nginx, API request logs, analytics or Referer headers.
function useEmailToken(): string {
  const [token] = useState(() => new URLSearchParams(window.location.hash.slice(1)).get('token') ?? '');
  useEffect(() => {
    window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search);
  }, []);
  return token;
}

export function ForgotPassword(): JSX.Element {
  useSeo('Сброс пароля — skillex.space');
  const [email, setEmail] = useState('');
  const mutation = useMutation({ mutationFn: () => forgotPassword(email.trim()) });
  const submit = (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); mutation.mutate(); };
  return <main className="wrap"><section className="page panel account-recovery">
    <h1>Сброс пароля</h1>
    <p>Укажите email аккаунта. Мы отправим ссылку для установки нового пароля.</p>
    {mutation.isSuccess ? <p role="status">{mutation.data.message}</p> : <form className="form-grid" onSubmit={submit}>
      <label htmlFor="recovery-email">Email</label>
      <input id="recovery-email" className="input" type="email" autoComplete="email" maxLength={254} required value={email} onChange={event => setEmail(event.target.value)} />
      <button className="btn btn-primary" type="submit" disabled={mutation.isPending}>{mutation.isPending ? 'Отправляем…' : 'Отправить ссылку'}</button>
    </form>}
    {mutation.isError && <p role="alert" className="error">{errorMessage(mutation.error)}</p>}
    <p><Link to="/login">Вернуться ко входу</Link></p>
  </section></main>;
}

export function ResetPassword(): JSX.Element {
  useSeo('Новый пароль — skillex.space');
  const token = useEmailToken();
  const client = useQueryClient();
  const [password, setPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [validation, setValidation] = useState('');
  const mutation = useMutation({ mutationFn: () => resetPassword(token, password), onSuccess: async () => { setPassword(''); setConfirmation(''); await setSessionUser(client, null); } });
  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    if ([...password].length < 8) { setValidation('Пароль должен содержать минимум 8 символов.'); return; }
    if (new TextEncoder().encode(password).length > 72) { setValidation('Пароль слишком длинный. Сократите его.'); return; }
    if (password !== confirmation) { setValidation('Пароли не совпадают.'); return; }
    setValidation(''); mutation.mutate();
  }
  return <main className="wrap"><section className="page panel account-recovery">
    <h1>Новый пароль</h1>
    {mutation.isSuccess ? <p role="status">{mutation.data.message}</p> : token ? <form className="form-grid" onSubmit={submit}>
      <p>После смены пароля все устройства будут отключены от аккаунта.</p>
      <label htmlFor="new-password">Новый пароль</label>
      <input id="new-password" className="input" type="password" autoComplete="new-password" required minLength={8} maxLength={72} value={password} onChange={event => setPassword(event.target.value)} aria-describedby="password-rules" />
      <small id="password-rules">Не менее 8 символов. Не используйте пароль от другого сервиса.</small>
      <label htmlFor="confirm-password">Повторите пароль</label>
      <input id="confirm-password" className="input" type="password" autoComplete="new-password" required value={confirmation} onChange={event => setConfirmation(event.target.value)} />
      <button className="btn btn-primary" type="submit" disabled={mutation.isPending}>{mutation.isPending ? 'Сохраняем…' : 'Сохранить новый пароль'}</button>
    </form> : <p role="alert">В ссылке отсутствует код сброса. Откройте полную ссылку из письма или запросите новую.</p>}
    {validation && <p role="alert" className="error">{validation}</p>}
    {mutation.isError && <p role="alert" className="error">{errorMessage(mutation.error)}</p>}
    <p><Link to="/forgot-password">Запросить новую ссылку</Link> · <Link to="/login">Войти</Link></p>
  </section></main>;
}

export function VerifyEmail(): JSX.Element {
  useSeo('Подтверждение email — skillex.space');
  const token = useEmailToken();
  const client = useQueryClient();
  const mutation = useMutation({ mutationFn: () => verifyEmail(token), onSuccess: () => {
    void client.invalidateQueries({ queryKey: ['me'] });
    void client.invalidateQueries({ queryKey: ['account-security'] });
  } });
  return <main className="wrap"><section className="page panel account-recovery">
    <h1>Подтверждение email</h1>
    {mutation.isSuccess ? <p role="status">{mutation.data.message}</p> : token ? <>
      <p>Нажмите кнопку, чтобы подтвердить адрес, на который пришло письмо.</p>
      <button className="btn btn-primary" type="button" disabled={mutation.isPending} onClick={() => mutation.mutate()}>{mutation.isPending ? 'Подтверждаем…' : 'Подтвердить email'}</button>
    </> : <p role="alert">В ссылке отсутствует код подтверждения. Откройте полную ссылку из письма или запросите новую в профиле.</p>}
    {mutation.isError && <p role="alert" className="error">{errorMessage(mutation.error)}</p>}
    <p><Link to="/profile">Перейти в профиль</Link> · <Link to="/login">Войти</Link></p>
  </section></main>;
}
