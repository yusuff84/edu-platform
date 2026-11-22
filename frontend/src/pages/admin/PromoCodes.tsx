import { useMemo, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { createPromoCode, deactivatePromoCode } from '../../api/client';
import { listAdminPromoCodes, overviewQueryKey, type AdminPromoCode } from '../../api/adminHub';
import { listAllAdminCourses, salesCoursesKey, useUrlFilters, useUrlSearch } from '../../api/adminSales';
import {
  AdminPage,
  Badge,
  Card,
  DataTable,
  EmptyState,
  ErrorNote,
  Field,
  FilterSelect,
  Icon,
  Modal,
  Pager,
  RowMenu,
  SearchInput,
  Segmented,
  Spinner,
  StatCard,
  Toolbar,
  errorText,
  fmtDate,
  fmtMoney,
  useAdminUi,
  type Column,
  type MenuItem,
  type Tone,
} from '../../components/admin/ui';
import { CourseCell, CoursePicker } from './Enrollments';
import '../../styles/admin-sales.css';

const LIST_KEY = ['admin', 'promo-codes'] as const;
const LIMIT = 25;
const CODE_RE = /^[A-Z0-9_-]{3,32}$/;

type PromoState = 'active' | 'disabled' | 'expired' | 'exhausted';
const STATE: Record<PromoState, { label: string; tone: Tone }> = {
  active: { label: 'Активен', tone: 'ok' },
  disabled: { label: 'Отключён', tone: 'neutral' },
  expired: { label: 'Истёк', tone: 'warn' },
  exhausted: { label: 'Исчерпан', tone: 'info' },
};

type StateFilter = '' | PromoState;

function promoState(p: AdminPromoCode): PromoState {
  if (!p.active) return 'disabled';
  if (p.expiresAt && new Date(p.expiresAt).getTime() < Date.now()) return 'expired';
  if (p.maxUses > 0 && p.usedCount >= p.maxUses) return 'exhausted';
  return 'active';
}

function isState(v: string): v is PromoState {
  return v === 'active' || v === 'disabled' || v === 'expired' || v === 'exhausted';
}

/** Цена со скидкой — как считает backend: скидка округляется вниз в пользу ученика. */
function discounted(price: number, pct: number): number {
  return price - Math.floor((price * pct) / 100);
}

function randomCode(): string {
  const abc = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  const buf = new Uint32Array(6);
  crypto.getRandomValues(buf);
  return `SALE-${Array.from(buf, (n) => abc[n % abc.length]).join('')}`;
}

function copy(text: string, toast: (t: string, tone?: 'ok' | 'error' | 'info') => void): void {
  void navigator.clipboard
    ?.writeText(text)
    .then(() => toast(`Скопировано: ${text}`))
    .catch(() => toast('Не удалось скопировать', 'error'));
}

function Usage({ p }: { p: AdminPromoCode }): JSX.Element {
  if (p.maxUses <= 0) {
    return (
      <span className="asl-progress-meta">
        <b className="ax-num">{p.usedCount}</b> · без лимита
      </span>
    );
  }
  const pct = Math.min(100, Math.round((p.usedCount / p.maxUses) * 100));
  return (
    <div className="asl-progress">
      <div className={`asl-progress-bar${pct >= 100 ? ' is-full' : ''}`} role="progressbar" aria-valuemin={0} aria-valuemax={p.maxUses} aria-valuenow={p.usedCount} aria-label="Использовано">
        <span style={{ width: `${pct}%` }} />
      </div>
      <span className="asl-progress-meta">
        {p.usedCount} из {p.maxUses}
      </span>
    </div>
  );
}

/** Модалка создания промокода. */
function CreateModal({ open, onClose, presetCourseId }: { open: boolean; onClose: () => void; presetCourseId: string }): JSX.Element {
  const qc = useQueryClient();
  const { toast } = useAdminUi();
  const courses = useQuery({ queryKey: salesCoursesKey, queryFn: listAllAdminCourses, staleTime: 60_000, enabled: open });
  const [courseId, setCourseId] = useState(presetCourseId);
  const [code, setCode] = useState('');
  const [discount, setDiscount] = useState('20');
  const [maxUses, setMaxUses] = useState('0');
  const [expires, setExpires] = useState('');
  const [touched, setTouched] = useState(false);

  const paid = (c: { priceCents: number; status: string }): boolean => c.priceCents > 0 && c.status !== 'archived';
  const course = courses.data?.find((c) => c.id === courseId);
  const pct = Number(discount);
  const uses = Number(maxUses);
  const today = new Date().toISOString().slice(0, 10);

  const errors = {
    course: !courseId || (course && !paid(course)) ? 'Выберите платный курс' : undefined,
    code: !CODE_RE.test(code) ? '3–32 символа: латинские буквы, цифры, дефис или подчёркивание' : undefined,
    discount: !Number.isInteger(pct) || pct < 1 || pct > 90 ? 'Целое число от 1 до 90' : undefined,
    uses: !Number.isInteger(uses) || uses < 0 ? 'Целое число, 0 — без лимита' : undefined,
    expires: expires && expires < today ? 'Дата должна быть не раньше сегодняшней' : undefined,
  };
  const hasErrors = Object.values(errors).some(Boolean);

  const create = useMutation({
    mutationFn: () =>
      createPromoCode(courseId, {
        code,
        discountPct: pct,
        maxUses: uses,
        expiresAt: expires ? new Date(`${expires}T23:59:59`).toISOString() : null,
      }),
    onSuccess: (p) => {
      toast(`Промокод ${p.code} создан`);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
      close();
    },
  });

  function close(): void {
    setCode('');
    setDiscount('20');
    setMaxUses('0');
    setExpires('');
    setTouched(false);
    create.reset();
    onClose();
  }

  function submit(e: FormEvent): void {
    e.preventDefault();
    setTouched(true);
    if (!hasErrors) create.mutate();
  }

  const err = (k: keyof typeof errors): string | undefined => (touched ? errors[k] : undefined);

  return (
    <Modal
      open={open}
      onClose={close}
      title="Новый промокод"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={close}>
            Отмена
          </button>
          <button type="submit" form="asl-promo-form" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? 'Создаём…' : 'Создать промокод'}
          </button>
        </>
      }
    >
      <form id="asl-promo-form" className="ax-form" onSubmit={submit} noValidate>
        <CoursePicker
          courses={courses.data ?? []}
          loading={courses.isLoading}
          value={courseId}
          onChange={(id) => {
            setCourseId(id);
            create.reset();
          }}
          filter={paid}
          hint="Только платные курсы не в архиве"
          error={err('course')}
        />
        <Field label="Код" hint="Ученик вводит его при оплате. Регистр не важен." error={err('code')}>
          <span className="asl-inline-input">
            <input
              className="ax-input ax-mono"
              value={code}
              maxLength={32}
              autoComplete="off"
              autoCapitalize="characters"
              placeholder="SPRING-25"
              onChange={(e) => {
                setCode(e.target.value.toUpperCase().replace(/\s+/g, ''));
                create.reset();
              }}
            />
            <button
              type="button"
              className="btn btn-ghost"
              onClick={(e) => {
                e.preventDefault();
                setCode(randomCode());
              }}
            >
              Сгенерировать
            </button>
          </span>
        </Field>
        <div className="ax-form-row">
          <Field label="Скидка, %" hint="от 1 до 90" error={err('discount')}>
            <input className="ax-input" type="number" inputMode="numeric" min={1} max={90} value={discount} onChange={(e) => setDiscount(e.target.value)} />
          </Field>
          <Field label="Лимит использований" hint="0 — без лимита" error={err('uses')}>
            <input className="ax-input" type="number" inputMode="numeric" min={0} value={maxUses} onChange={(e) => setMaxUses(e.target.value)} />
          </Field>
        </div>
        <Field label="Действует до (необязательно)" hint="Включительно. Пусто — бессрочно." error={err('expires')}>
          <input className="ax-input" type="date" min={today} value={expires} onChange={(e) => setExpires(e.target.value)} />
        </Field>
        {course && paid(course) && !errors.discount && (
          <div className="asl-preview" aria-live="polite">
            <span className="ax-muted">Цена для ученика:</span>
            <span className="asl-old-price">{fmtMoney(course.priceCents)}</span>
            <strong>{fmtMoney(discounted(course.priceCents, pct))}</strong>
          </div>
        )}
        {create.isError && <ErrorNote error={create.error} />}
      </form>
    </Modal>
  );
}

export default function PromoCodes(): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const { get, set } = useUrlFilters();
  const q = get('q');
  const courseId = get('courseId');
  const stateParam = get('status');
  const stateFilter: StateFilter = isState(stateParam) ? stateParam : '';
  const page = Math.max(1, Number(get('page')) || 1);
  const createOpen = get('new') === '1';
  const [search, setSearch] = useUrlSearch('q', q, set);

  const list = useQuery({ queryKey: LIST_KEY, queryFn: () => listAdminPromoCodes() });
  const all = list.data?.items ?? [];

  const counts = useMemo(() => {
    const c: Record<PromoState, number> = { active: 0, disabled: 0, expired: 0, exhausted: 0 };
    for (const p of all) if (!courseId || p.courseId === courseId) c[promoState(p)] += 1;
    return c;
  }, [all, courseId]);

  const filtered = useMemo(() => {
    const needle = q.trim().toUpperCase();
    return all.filter(
      (p) =>
        (!courseId || p.courseId === courseId) &&
        (!stateFilter || promoState(p) === stateFilter) &&
        (!needle || p.code.includes(needle) || p.courseTitle.toUpperCase().includes(needle)),
    );
  }, [all, courseId, stateFilter, q]);
  const rows = filtered.slice((page - 1) * LIMIT, page * LIMIT);

  const courseOptions = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of all) m.set(p.courseId, p.courseTitle);
    const opts = [...m.entries()].sort((a, b) => a[1].localeCompare(b[1], 'ru')).map(([value, label]) => ({ value, label }));
    if (courseId && !m.has(courseId)) opts.push({ value: courseId, label: 'Выбранный курс' });
    return [{ value: '', label: 'Все курсы' }, ...opts];
  }, [all, courseId]);

  const totalUses = all.reduce((s, p) => s + p.usedCount, 0);

  const deactivate = useMutation({
    mutationFn: (p: AdminPromoCode) => deactivatePromoCode(p.courseId, p.id),
    onSuccess: (_r, p) => {
      toast(`Промокод ${p.code} отключён`);
      void qc.invalidateQueries({ queryKey: LIST_KEY });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (e) => toast(errorText(e), 'error'),
  });

  async function onDeactivate(p: AdminPromoCode): Promise<void> {
    const ok = await confirm({
      title: `Отключить промокод ${p.code}?`,
      text: (
        <p>
          Курс «{p.courseTitle}». Код перестанет действовать при оплате. История использований сохранится, но включить код обратно
          нельзя — при необходимости создайте новый.
        </p>
      ),
      confirmLabel: 'Отключить',
      danger: true,
    });
    if (ok) deactivate.mutate(p);
  }

  const menu = (p: AdminPromoCode): MenuItem[] => [
    { label: 'Скопировать код', icon: 'copy', onClick: () => copy(p.code, toast) },
    { label: 'Открыть курс', icon: 'book', to: `/admin/courses/${p.courseId}` },
    { label: 'Платежи по курсу', icon: 'card', to: `/admin/payments?courseId=${p.courseId}` },
    'divider',
    { label: p.active ? 'Отключить' : 'Уже отключён', icon: 'lock', danger: p.active, disabled: !p.active, onClick: () => void onDeactivate(p) },
  ];

  const columns: Column<AdminPromoCode>[] = [
    {
      key: 'code',
      header: 'Код',
      mobile: 'title',
      render: (p) => (
        <span className="asl-code-cell">
          <span className="asl-code">{p.code}</span>
          <button type="button" className="ax-icon-btn" aria-label={`Скопировать ${p.code}`} onClick={() => copy(p.code, toast)}>
            <Icon name="copy" size={16} />
          </button>
        </span>
      ),
    },
    { key: 'course', header: 'Курс', render: (p) => <CourseCell id={p.courseId} title={p.courseTitle} /> },
    { key: 'discount', header: 'Скидка', align: 'right', render: (p) => <span className="ax-num">−{p.discountPct} %</span> },
    {
      key: 'price',
      header: 'Цена',
      align: 'right',
      render: (p) =>
        p.coursePriceCents > 0 ? (
          <span className="asl-amount">
            <span className="ax-num asl-money">{fmtMoney(discounted(p.coursePriceCents, p.discountPct))}</span>
            <span className="asl-old-price">{fmtMoney(p.coursePriceCents)}</span>
          </span>
        ) : (
          <span className="ax-muted">курс бесплатный</span>
        ),
    },
    { key: 'uses', header: 'Использовано', render: (p) => <Usage p={p} /> },
    {
      key: 'expires',
      header: 'Срок',
      render: (p) => (p.expiresAt ? <span className="asl-nowrap">до {fmtDate(p.expiresAt)}</span> : <span className="ax-muted asl-nowrap">бессрочно</span>),
    },
    {
      key: 'state',
      header: 'Статус',
      render: (p) => {
        const s = STATE[promoState(p)];
        return <Badge tone={s.tone}>{s.label}</Badge>;
      },
    },
  ];

  const filtersActive = Boolean(q || courseId || stateFilter);
  const openCreate = (): void => set({ new: '1' }, { replace: true });

  return (
    <AdminPage
      title="Промокоды"
      description="Скидочные коды всех платных курсов: кто сколько раз ими воспользовался, сроки и лимиты."
      actions={
        <button type="button" className="btn btn-primary" onClick={openCreate}>
          <Icon name="plus" size={17} /> Создать промокод
        </button>
      }
    >
      <div className="ax-grid ax-grid--3 asl-stats">
        <StatCard label="Активных" icon="tag" tone="ok" value={list.data ? counts.active : '—'} hint={courseId ? 'по выбранному курсу' : 'по всем курсам'} />
        <StatCard label="Всего кодов" icon="history" tone="neutral" value={list.data ? all.length : '—'} hint={`отключённых, истёкших и исчерпанных: ${counts.disabled + counts.expired + counts.exhausted}`} />
        <StatCard label="Применено" icon="card" tone="brand" value={list.data ? totalUses : '—'} hint="оплат с промокодом за всё время" />
      </div>

      <Card flush className="asl-card">
        <div className="asl-toolbar-stack">
          <Toolbar
            end={
              filtersActive ? (
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => {
                    setSearch('');
                    set({ q: null, courseId: null, status: null });
                  }}
                >
                  Сбросить
                </button>
              ) : undefined
            }
          >
            <SearchInput value={search} onChange={setSearch} placeholder="Поиск по коду или курсу" />
            <FilterSelect label="Курс" value={courseId} onChange={(v) => set({ courseId: v || null })} options={courseOptions} />
          </Toolbar>
          <Segmented<StateFilter>
            label="Статус промокода"
            value={stateFilter}
            onChange={(v) => set({ status: v || null })}
            options={[
              { value: '', label: 'Все' },
              { value: 'active', label: 'Активные', count: counts.active },
              { value: 'disabled', label: 'Отключённые', count: counts.disabled },
              { value: 'expired', label: 'Истёкшие', count: counts.expired },
              { value: 'exhausted', label: 'Исчерпанные', count: counts.exhausted },
            ]}
          />
        </div>
        {list.isLoading ? (
          <Spinner />
        ) : list.isError ? (
          <ErrorNote error={list.error} onRetry={() => void list.refetch()} />
        ) : (
          <>
            <DataTable
              columns={columns}
              rows={rows}
              rowKey={(p) => p.id}
              actions={(p) => <RowMenu items={menu(p)} />}
              caption="Промокоды"
              empty={
                filtersActive ? (
                  <EmptyState icon="search" title="Ничего не найдено" text="Измените поиск или фильтры." />
                ) : (
                  <EmptyState
                    icon="tag"
                    title="Промокодов пока нет"
                    text="Создайте код со скидкой для платного курса — например, для рассылки или партнёров."
                    action={
                      <button type="button" className="btn btn-primary" onClick={openCreate}>
                        Создать промокод
                      </button>
                    }
                  />
                )
              }
            />
            <Pager page={page} total={filtered.length} limit={LIMIT} onPage={(p) => set({ page: String(p) })} />
          </>
        )}
      </Card>

      {createOpen && <CreateModal open onClose={() => set({ new: null }, { replace: true })} presetCourseId={courseId} />}
    </AdminPage>
  );
}
