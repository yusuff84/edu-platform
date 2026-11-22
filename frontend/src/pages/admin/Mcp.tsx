import { useState, type FormEvent, type ReactNode } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminCreateMcpToken, adminListMcpTokens, adminRevokeMcpToken, type McpToken } from '../../api/adminOps';
import { overviewQueryKey } from '../../api/adminHub';
import {
  AdminPage,
  Badge,
  Card,
  CopyButton,
  DataTable,
  EmptyState,
  ErrorNote,
  Field,
  Icon,
  Note,
  RowMenu,
  Segmented,
  Spinner,
  Tabs,
  errorText,
  fmtAgo,
  fmtDate,
  fmtDateTime,
  useAdminUi,
  type Column,
} from '../../components/admin/ui';
import '../../styles/admin-system.css';

type Client = 'claude' | 'json';
type Show = 'active' | 'revoked' | 'all';

const tokensKey = ['admin-mcp-tokens'] as const;

const EXAMPLES = [
  'Покажи список всех курсов на платформе со статусами и ценами.',
  'Создай в skillex курс «Excel для начинающих»: 6 модулей, в каждом 4 урока с конспектом и тест в конце модуля.',
  'Открой курс «Python с нуля» и добавь в модуль 2 урок про циклы с примерами кода.',
  'Проверь курс «Основы SQL»: найди опечатки в уроках и исправь их.',
  'Сними курс «Figma за неделю» с публикации, перепиши описание и опубликуй снова.',
];

const TOOLS = [
  ['list_courses / get_course', 'посмотреть курсы и программу'],
  ['create_course / update_course', 'создать курс, изменить название, описание, цену'],
  ['add_module / rename_module / delete_module', 'модули программы'],
  ['add_lesson / update_lesson / delete_lesson', 'уроки: текст, видео, тесты'],
  ['publish_course / unpublish_course', 'опубликовать или снять с публикации'],
  ['delete_course', 'удалить черновик без учеников'],
];

function CodeBlock({ code, copyLabel = 'Скопировать', block }: { code: string; copyLabel?: string; block?: boolean }): JSX.Element {
  return (
    <div className={`asy-codebox${block ? ' asy-codebox--block' : ''}`}>
      {block ? <pre className="asy-pre">{code}</pre> : <code className="asy-code">{code}</code>}
      <CopyButton text={code} label={copyLabel} />
    </div>
  );
}

function Step({ n, title, children, done }: { n: number; title: string; children: ReactNode; done?: boolean }): JSX.Element {
  return (
    <section className="ax-card asy-step">
      <div className="asy-step-head">
        <span className={`asy-step-num${done ? ' is-done' : ''}`} aria-hidden="true">
          {done ? <Icon name="check" size={16} /> : n}
        </span>
        <h2 className="ax-card-title">
          <span className="sr-only">Шаг {n}. </span>
          {title}
        </h2>
      </div>
      <div className="asy-step-body">{children}</div>
    </section>
  );
}

export default function Mcp(): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const [params, setParams] = useSearchParams();
  const client: Client = params.get('client') === 'json' ? 'json' : 'claude';
  const show: Show = params.get('show') === 'revoked' ? 'revoked' : params.get('show') === 'all' ? 'all' : 'active';
  const setParam = (k: string, v: string): void =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (v) next.set(k, v);
        else next.delete(k);
        return next;
      },
      { replace: true },
    );

  const [name, setName] = useState('');
  const [token, setToken] = useState('');
  const [tokenName, setTokenName] = useState('');

  const list = useQuery({ queryKey: tokensKey, queryFn: adminListMcpTokens });

  const create = useMutation({
    mutationFn: (n: string) => adminCreateMcpToken(n),
    onSuccess: (r) => {
      setToken(r.token);
      window.setTimeout(() => document.querySelector('.asy-created')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }), 50);
      setTokenName(r.item.name);
      setName('');
      toast('Токен выпущен — скопируйте его сейчас');
      void qc.invalidateQueries({ queryKey: tokensKey });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => adminRevokeMcpToken(id),
    onSuccess: () => {
      toast('Токен отозван — нейросеть с ним больше не подключится');
      void qc.invalidateQueries({ queryKey: tokensKey });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  async function onRevoke(t: McpToken): Promise<void> {
    const ok = await confirm({
      title: `Отозвать токен «${t.name}»?`,
      text: 'Нейросеть, которая подключена с этим токеном, сразу потеряет доступ. Вернуть токен нельзя — понадобится выпустить новый.',
      confirmLabel: 'Отозвать токен',
      danger: true,
    });
    if (ok) revoke.mutate(t.id);
  }

  function onCreate(e: FormEvent): void {
    e.preventDefault();
    create.mutate(name.trim() || 'Нейросеть');
  }

  const url = `${window.location.origin}/api/v1/mcp`;
  const shown = token || '<ВАШ_ТОКЕН>';
  const claudeCmd = `claude mcp add --transport http skillex ${url} --header "Authorization: Bearer ${shown}"`;
  const jsonCfg = JSON.stringify(
    { mcpServers: { skillex: { type: 'http', url, headers: { Authorization: `Bearer ${shown}` } } } },
    null,
    2,
  );

  const items = list.data?.items ?? [];
  const activeCount = items.filter((t) => !t.revokedAt).length;
  const revokedCount = items.length - activeCount;
  const rows = items.filter((t) => (show === 'all' ? true : show === 'active' ? !t.revokedAt : Boolean(t.revokedAt)));

  const columns: Column<McpToken>[] = [
    {
      key: 'name',
      header: 'Токен',
      mobile: 'title',
      render: (t) => (
        <div className="ax-person">
          <span className={`asy-token-icon${t.revokedAt ? ' is-off' : ''}`} aria-hidden="true">
            <Icon name="key" size={18} />
          </span>
          <div className="ax-person-text">
            <strong>{t.name}</strong>
            <small className="ax-mono">{t.prefix}…</small>
          </div>
        </div>
      ),
    },
    { key: 'who', header: 'Кто выпустил', render: (t) => <span className="asy-break">{t.userEmail}</span> },
    { key: 'created', header: 'Создан', width: '150px', render: (t) => <span className="asy-nowrap" title={fmtDateTime(t.createdAt)}>{fmtDate(t.createdAt)}</span> },
    {
      key: 'used',
      header: 'Последний вызов',
      width: '160px',
      render: (t) =>
        t.lastUsedAt ? <span title={fmtDateTime(t.lastUsedAt)}>{fmtAgo(t.lastUsedAt)}</span> : <span className="ax-muted">ещё не было</span>,
    },
    {
      key: 'status',
      header: 'Статус',
      width: '120px',
      render: (t) =>
        t.revokedAt ? (
          <span title={`Отозван ${fmtDateTime(t.revokedAt)}`}>
            <Badge tone="neutral">Отозван</Badge>
          </span>
        ) : (
          <Badge tone="ok">Активен</Badge>
        ),
    },
  ];

  return (
    <AdminPage
      title="Нейросеть (MCP)"
      description="Подключите Claude, Cursor или другую нейросеть с поддержкой MCP — она сможет создавать и править курсы, модули, уроки и тесты по вашим просьбам обычным текстом."
    >
      <Note tone="info">
        <div className="asy-security">
          <Icon name="shield" size={18} />
          <span>
            <b>Безопасно:</b> доступ только по секретному токену администратора, в базе хранится лишь его хеш, соединение
            шифруется. Токен можно отозвать в любой момент. Всё, что нейросеть меняет, записывается в{' '}
            <Link className="ax-link" to="/admin/audit?q=MCP">
              журнал действий
            </Link>
            .
          </span>
        </div>
      </Note>

      <div className="asy-steps">
        <Step n={1} title="Выпустите токен" done={Boolean(token)}>
          <p className="ax-muted asy-p">
            Токен — это пароль для нейросети. Дайте ему понятное название, чтобы потом знать, где он используется.
          </p>
          <form className="asy-invite-form" onSubmit={onCreate}>
            <Field label="Название токена" hint="Например: «Claude — рабочий ноутбук»">
              <input
                className="ax-input"
                placeholder="Нейросеть"
                value={name}
                maxLength={60}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <button type="submit" className="btn btn-primary asy-invite-submit" disabled={create.isPending}>
              <Icon name="key" size={17} /> {create.isPending ? 'Выпускаем…' : 'Выпустить токен'}
            </button>
          </form>

          {token && (
            <div className="asy-created asy-created--warn" role="status">
              <div className="asy-created-head">
                <span className="asy-created-icon asy-created-icon--warn">
                  <Icon name="alert" size={18} />
                </span>
                <div>
                  <strong>Токен «{tokenName}» показывается только сейчас</strong>
                  <p className="ax-muted">
                    Скопируйте его и сохраните в надёжном месте — повторно увидеть его нельзя. Никому не пересылайте токен в
                    открытых чатах. Команды ниже уже содержат его.
                  </p>
                </div>
              </div>
              <CodeBlock code={token} copyLabel="Скопировать токен" />
              <button type="button" className="btn btn-ghost btn-sm asy-hide-token" onClick={() => setToken('')}>
                Я сохранил токен — скрыть
              </button>
            </div>
          )}
        </Step>

        <Step n={2} title="Подключите нейросеть">
          <Tabs<Client>
            label="Программа"
            value={client}
            onChange={(v) => setParam('client', v === 'claude' ? '' : v)}
            tabs={[
              { id: 'claude', label: 'Claude Code' },
              { id: 'json', label: 'Cursor и другие' },
            ]}
          />
          {client === 'claude' ? (
            <div className="asy-stack">
              <p className="asy-p">Выполните команду в терминале — один раз на каждом компьютере:</p>
              <CodeBlock code={claudeCmd} copyLabel="Скопировать команду" />
            </div>
          ) : (
            <div className="asy-stack">
              <p className="asy-p">
                Добавьте в настройки MCP-серверов (файл <span className="ax-mono">mcp.json</span> в Cursor, Windsurf, Claude Desktop и
                т. п.):
              </p>
              <CodeBlock code={jsonCfg} copyLabel="Скопировать настройки" block />
            </div>
          )}
          {!token && (
            <p className="ax-muted asy-p asy-small">
              Вместо <span className="ax-mono">&lt;ВАШ_ТОКЕН&gt;</span> подставьте токен из шага 1 — или выпустите новый, и он подставится сам.
            </p>
          )}
          <div className="asy-server">
            <span className="ax-muted">Адрес сервера:</span>
            <code className="asy-code">{url}</code>
            <CopyButton text={url} label="Скопировать адрес" />
          </div>
        </Step>

        <Step n={3} title="Что можно попросить">
          <p className="ax-muted asy-p">Пишите нейросети обычным языком. Например:</p>
          <ul className="asy-examples">
            {EXAMPLES.map((ex) => (
              <li key={ex}>
                <Icon name="message" size={16} />
                <span>{ex}</span>
                <CopyButton text={ex} label="Копировать" />
              </li>
            ))}
          </ul>
          <details className="asy-details">
            <summary>Какие инструменты видит нейросеть</summary>
            <dl className="asy-tools">
              {TOOLS.map(([k, v]) => (
                <div key={k}>
                  <dt className="ax-mono">{k}</dt>
                  <dd>{v}</dd>
                </div>
              ))}
            </dl>
          </details>
        </Step>
      </div>

      <Card
        flush
        title="Выпущенные токены"
        description="Если токен больше не нужен или мог попасть в чужие руки — отзовите его."
        actions={
          revokedCount > 0 ? (
            <Segmented<Show>
              label="Какие токены показать"
              value={show}
              onChange={(v) => setParam('show', v === 'active' ? '' : v)}
              options={[
                { value: 'active', label: 'Активные', count: activeCount },
                { value: 'revoked', label: 'Отозванные', count: revokedCount },
                { value: 'all', label: 'Все', count: items.length },
              ]}
            />
          ) : undefined
        }
      >
        {list.isLoading && <Spinner />}
        {list.isError && <ErrorNote error={list.error} onRetry={() => void list.refetch()} />}
        {list.data && (
          <DataTable
            caption="MCP-токены"
            columns={columns}
            rows={rows}
            rowKey={(t) => t.id}
            empty={
              <EmptyState
                icon="key"
                title={items.length === 0 ? 'Токенов пока нет' : show === 'active' ? 'Активных токенов нет' : 'Ничего нет'}
                text={items.length === 0 || show === 'active' ? 'Выпустите токен в шаге 1, чтобы подключить нейросеть.' : undefined}
              />
            }
            actions={(t) => (
              <RowMenu
                label={`Действия с токеном ${t.name}`}
                items={[
                  { label: 'Действия нейросети в журнале', icon: 'history', to: `/admin/audit?q=${encodeURIComponent('MCP')}` },
                  ...(t.revokedAt
                    ? []
                    : [
                        'divider' as const,
                        {
                          label: 'Отозвать токен',
                          icon: 'x' as const,
                          danger: true,
                          disabled: revoke.isPending && revoke.variables === t.id,
                          onClick: () => void onRevoke(t),
                        },
                      ]),
                ]}
              />
            )}
          />
        )}
      </Card>
    </AdminPage>
  );
}
