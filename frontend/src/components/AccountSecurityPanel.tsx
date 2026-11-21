import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getAccountSecurity, requestEmailVerification, getAccountSessions, revokeAccountSession, revokeOtherSessions, type AccountSession } from '../api/accountSecurity';
import { formatDate } from '../api/client';
import { setSessionUser } from '../app/sessionCache';
import '../styles/accountSecurity.css';

function deviceName(agent: string): string {
  const browser = /Edg\//.test(agent) ? 'Edge' : /Firefox\//.test(agent) ? 'Firefox' : /Chrome\//.test(agent) ? 'Chrome' : /Safari\//.test(agent) ? 'Safari' : 'Браузер';
  const system = /Android/.test(agent) ? 'Android' : /iPhone|iPad/.test(agent) ? 'iOS' : /Windows/.test(agent) ? 'Windows' : /Macintosh/.test(agent) ? 'macOS' : /Linux/.test(agent) ? 'Linux' : '';
  return system ? `${browser} · ${system}` : browser;
}
const message = (error: unknown) => error instanceof Error ? error.message : 'Не удалось выполнить запрос.';

export default function AccountSecurityPanel(): JSX.Element {
  const client = useQueryClient();
  const navigate = useNavigate();
  const [sessionMessage, setSessionMessage] = useState('');
  const security = useQuery({ queryKey: ['account-security'], queryFn: getAccountSecurity, retry: false });
  const sessions = useQuery({ queryKey: ['account-sessions'], queryFn: getAccountSessions, retry: false });
  const verification = useMutation({ mutationFn: requestEmailVerification });
  const revoke = useMutation({ mutationFn: (session: AccountSession) => revokeAccountSession(session.id), onSuccess: async (_, session) => {
    if (session.current) { await setSessionUser(client, null); navigate('/login', { replace: true }); }
    else { setSessionMessage('Сессия завершена.'); await client.invalidateQueries({ queryKey: ['account-sessions'] }); }
  } });
  const others = useMutation({ mutationFn: revokeOtherSessions, onSuccess: async () => {
    setSessionMessage('Все другие сессии завершены.'); await client.invalidateQueries({ queryKey: ['account-sessions'] });
  } });
  const busy = revoke.isPending || others.isPending;
  return <>
    <section className="panel account-security" aria-labelledby="account-security-title">
      <h2 id="account-security-title">Безопасность аккаунта</h2>
      {security.isLoading && <p role="status">Загружаем настройки…</p>}
      {security.isError && <div role="alert"><p className="error">{message(security.error)}</p><button className="btn" type="button" onClick={() => void security.refetch()}>Повторить</button></div>}
      {security.data && <>
        <p>Email: <strong>{security.data.email}</strong> — <span>{security.data.emailVerified ? 'Подтверждён' : 'Не подтверждён'}</span></p>
        {!security.data.emailVerified && <>
          <p>Подтвердите email, чтобы убедиться, что важные письма приходят вам.</p>
          {security.data.emailDeliveryAvailable ? <button className="btn" type="button" disabled={verification.isPending} onClick={() => verification.mutate()}>{verification.isPending ? 'Отправляем…' : 'Отправить письмо подтверждения'}</button> : <p className="muted">Отправка писем сейчас недоступна. Для подтверждения email обратитесь в поддержку.</p>}
        </>}
      </>}
      {verification.isSuccess && <p role="status">{verification.data.message}</p>}
      {verification.isError && <p role="alert" className="error">{message(verification.error)}</p>}
      <p><Link to="/forgot-password">Сбросить пароль</Link></p>
    </section>
    <section className="panel account-security" aria-labelledby="account-sessions-title">
      <h2 id="account-sessions-title">Активные сессии</h2>
      <p>Завершите незнакомые сессии. Если доступ к аккаунту мог получить другой человек, также смените пароль.</p>
      {sessions.isLoading && <p role="status">Загружаем сессии…</p>}
      {sessions.isError && <div role="alert"><p className="error">{message(sessions.error)}</p><button className="btn" type="button" onClick={() => void sessions.refetch()}>Повторить</button></div>}
      {sessions.data && <>
        <button className="btn" type="button" disabled={busy || !sessions.data.some(session => !session.current)} onClick={() => others.mutate()}>Завершить другие сессии</button>
        <ul className="account-session-list">
          {sessions.data.map(session => <li className="account-session" key={session.id}>
            <div><strong>{deviceName(session.userAgent)}</strong>{session.current && <span className="pill">Текущая сессия</span>}
              <p>IP: {session.ipAddress || 'Не указан'}</p>
              <p>Вход: {formatDate(session.createdAt)} · Активность: {formatDate(session.lastSeenAt)}</p>
              <p>Действует до: {formatDate(session.expiresAt)}</p>
            </div>
            <button className="btn" type="button" disabled={busy} onClick={() => revoke.mutate(session)}>{session.current ? 'Выйти с этого устройства' : 'Завершить сессию'}</button>
          </li>)}
        </ul>
      </>}
      {sessionMessage && <p role="status">{sessionMessage}</p>}
      {(revoke.isError || others.isError) && <p role="alert" className="error">{message(revoke.error ?? others.error)}</p>}
    </section>
  </>;
}
