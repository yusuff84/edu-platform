import { useState } from 'react';
import type { FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminCreateMcpToken, adminListMcpTokens, adminRevokeMcpToken } from '../api/adminOps';
import { formatDate } from '../api/client';

function Copy({ text }: { text: string }): JSX.Element {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="btn btn-sm"
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => {
          setDone(true);
          window.setTimeout(() => setDone(false), 1500);
        });
      }}
    >
      {done ? 'Скопировано' : 'Скопировать'}
    </button>
  );
}

/**
 * MCP-доступ для нейросети: выпуск токена (показывается один раз), готовые
 * команды подключения для Claude Code / Cursor и других клиентов, список
 * токенов с последним использованием и отзыв.
 */
export function AdminMcp(): JSX.Element {
  const qc = useQueryClient();
  const [name, setName] = useState('');
  const [token, setToken] = useState('');
  const list = useQuery({ queryKey: ['admin-mcp-tokens'], queryFn: adminListMcpTokens });
  const create = useMutation({
    mutationFn: () => adminCreateMcpToken(name.trim() || 'Нейросеть'),
    onSuccess: (r) => {
      setToken(r.token);
      setName('');
      void qc.invalidateQueries({ queryKey: ['admin-mcp-tokens'] });
    },
  });
  const revoke = useMutation({
    mutationFn: (id: string) => adminRevokeMcpToken(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin-mcp-tokens'] }),
  });

  const url = `${window.location.origin}/api/v1/mcp`;
  const shown = token || '<ВАШ_ТОКЕН>';
  const claudeCmd = `claude mcp add --transport http skillex ${url} --header "Authorization: Bearer ${shown}"`;
  const jsonCfg = JSON.stringify(
    { mcpServers: { skillex: { type: 'http', url, headers: { Authorization: `Bearer ${shown}` } } } },
    null,
    2,
  );

  function onCreate(e: FormEvent): void {
    e.preventDefault();
    create.mutate();
  }

  return (
    <section id="mcp" className="admin-section panel admin-mcp">
      <h2>MCP: курсы через нейросеть</h2>
      <p className="muted">
        Подключите Claude, Cursor или другую нейросеть с поддержкой MCP — она сможет создавать курсы, модули,
        уроки и тесты, править и публиковать их. Доступ только по секретному токену администратора: в базе
        хранится лишь его хеш, соединение шифруется HTTPS, токен можно отозвать в любой момент.
      </p>

      <form className="aua-inline" onSubmit={onCreate}>
        <input
          className="input"
          placeholder="Название токена, например «Claude — ноутбук»"
          value={name}
          maxLength={60}
          onChange={(e) => setName(e.target.value)}
          aria-label="Название токена"
        />
        <button type="submit" className="btn btn-sm btn-primary" disabled={create.isPending}>
          Выпустить токен
        </button>
      </form>
      {create.isError && <p className="error" role="alert">{create.error instanceof Error ? create.error.message : 'Ошибка'}</p>}

      {token && (
        <div className="mcp-token" role="status">
          <p>
            <b>Токен показывается только сейчас.</b> Сохраните его в надёжном месте — повторно увидеть его нельзя.
          </p>
          <div className="mcp-code">
            <code>{token}</code>
            <Copy text={token} />
          </div>
        </div>
      )}

      <div className="mcp-howto">
        <h3>Как подключить</h3>
        <p className="muted">Адрес сервера: <code>{url}</code></p>
        <p><b>Claude Code</b> — выполните в терминале:</p>
        <div className="mcp-code">
          <code>{claudeCmd}</code>
          <Copy text={claudeCmd} />
        </div>
        <p><b>Cursor и другие клиенты</b> — добавьте в настройки MCP (mcp.json):</p>
        <div className="mcp-code mcp-code--block">
          <pre>{jsonCfg}</pre>
          <Copy text={jsonCfg} />
        </div>
        <p className="muted">
          Затем попросите нейросеть, например: «Создай в skillex курс по Excel для начинающих: 6 модулей, в каждом 4
          урока и тест». Она увидит инструменты create_course, add_module, add_lesson, publish_course и другие.
        </p>
      </div>

      <h3>Выпущенные токены</h3>
      {list.data && list.data.items.length === 0 && <p className="muted">Токенов пока нет.</p>}
      <ul className="aua-courses">
        {(list.data?.items ?? []).map((t) => (
          <li key={t.id} className={`aua-course${t.revokedAt ? ' is-revoked' : ''}`}>
            <div className="aua-course-head">
              <b>{t.name}</b>
              <span className={`aua-status aua-status--${t.revokedAt ? 'revoked' : 'active'}`}>
                {t.revokedAt ? 'Отозван' : 'Активен'}
              </span>
            </div>
            <span className="muted aua-meta">
              <code>{t.prefix}…</code> · {t.userEmail} · выпущен {formatDate(t.createdAt)} ·{' '}
              {t.lastUsedAt ? `использован ${formatDate(t.lastUsedAt)}` : 'ещё не использовался'}
            </span>
            {!t.revokedAt && (
              <div className="aua-actions">
                <button
                  type="button"
                  className="btn btn-sm aua-danger"
                  disabled={revoke.isPending}
                  onClick={() => {
                    if (window.confirm(`Отозвать токен «${t.name}»? Нейросеть с ним сразу потеряет доступ.`)) revoke.mutate(t.id);
                  }}
                >
                  Отозвать
                </button>
              </div>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
