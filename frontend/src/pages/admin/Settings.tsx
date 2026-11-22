import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { useBlocker } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getAdminSettings, updateAdminSettings, updateAdminSettingsPromo } from '../../api/client';
import { adminGetSiteSettings, adminPutSiteSettings, type SiteSettings } from '../../api/adminOps';
import { overviewQueryKey } from '../../api/adminHub';
import {
  AdminPage,
  Card,
  ErrorNote,
  Field,
  Icon,
  Spinner,
  Switch,
  errorText,
  useAdminUi,
} from '../../components/admin/ui';
import '../../styles/admin-system.css';

type SiteKey = keyof SiteSettings;

/** Тексты по умолчанию — те же, что подставляет сайт (app/router.tsx), если поле пустое. */
const SITE_FIELDS: Record<SiteKey, { label: string; def: string; max: number; hint?: string }> = {
  promo_text: { label: 'Текст полосы', def: 'Начните учиться бесплатно — регистрация займёт минуту', max: 160 },
  promo_link_text: { label: 'Надпись на кнопке', def: 'Выбрать курс', max: 40 },
  promo_link_url: {
    label: 'Куда ведёт кнопка',
    def: '/',
    max: 300,
    hint: 'Страница сайта (например, /course/excel) или полный адрес, начинающийся с https://',
  },
  contact_phone: { label: 'Телефон', def: '8 (800) 600-78-47', max: 40 },
  contact_email: { label: 'Email', def: 'hello@edu-platform.ru', max: 120 },
  contact_city: { label: 'Город', def: 'Москва', max: 80 },
  footer_tagline: {
    label: 'Описание платформы',
    def: 'Маркетплейс IT-курсов для начинающих. Учим спокойно, проверяем честно, поддерживаем на каждом шаге.',
    max: 300,
  },
};
const SITE_KEYS = Object.keys(SITE_FIELDS) as SiteKey[];

type Form = { commission: string; promoOn: boolean; site: Record<SiteKey, string> };

function siteFrom(s: SiteSettings | undefined): Record<SiteKey, string> {
  return Object.fromEntries(SITE_KEYS.map((k) => [k, s?.[k] ?? ''])) as Record<SiteKey, string>;
}

function validate(f: Form): Partial<Record<'commission' | SiteKey, string>> {
  const errs: Partial<Record<'commission' | SiteKey, string>> = {};
  const raw = f.commission.trim().replace(',', '.');
  const pct = raw === '' ? NaN : Number(raw);
  if (!Number.isInteger(pct) || pct < 0 || pct > 90) errs.commission = 'Введите целое число от 0 до 90';
  const url = f.site.promo_link_url.trim();
  if (url && !((url.startsWith('/') && !url.startsWith('//')) || /^https:\/\/[^\s]+$/.test(url))) {
    errs.promo_link_url = 'Ссылка должна начинаться с / (страница сайта) или с https://';
  }
  const mail = f.site.contact_email.trim();
  if (mail && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(mail)) errs.contact_email = 'Проверьте адрес — похоже, в нём опечатка';
  for (const k of SITE_KEYS) {
    if (f.site[k].trim().length > SITE_FIELDS[k].max) errs[k] = `Не длиннее ${SITE_FIELDS[k].max} символов`;
  }
  return errs;
}

export default function Settings(): JSX.Element {
  const qc = useQueryClient();
  const { confirm, toast } = useAdminUi();
  const platform = useQuery({ queryKey: ['admin-settings'], queryFn: getAdminSettings });
  const site = useQuery({ queryKey: ['admin-site-settings'], queryFn: adminGetSiteSettings });

  const initial = useMemo<Form | null>(() => {
    if (!platform.data || !site.data) return null;
    return {
      commission: String(platform.data.commissionPct),
      promoOn: platform.data.promoBarEnabled,
      site: siteFrom(site.data.settings),
    };
  }, [platform.data, site.data]);

  const [form, setForm] = useState<Form | null>(null);
  const [showErrors, setShowErrors] = useState(false);
  const [saving, setSaving] = useState(false);

  // Новые данные с сервера подставляем, только если пользователь ничего не правил
  // (фоновое обновление не должно стирать введённое).
  const prevInitial = useRef<Form | null>(null);
  useEffect(() => {
    if (!initial) return;
    setForm((f) => (f === null || f === prevInitial.current ? initial : f));
    prevInitial.current = initial;
  }, [initial]);

  const changed = useMemo(() => {
    if (!form || !initial) return { commission: false, promo: false, site: false, any: false };
    const commission = form.commission.trim() !== initial.commission;
    const promo = form.promoOn !== initial.promoOn;
    const siteChanged = SITE_KEYS.some((k) => form.site[k].trim() !== initial.site[k].trim());
    return { commission, promo, site: siteChanged, any: commission || promo || siteChanged };
  }, [form, initial]);

  const errors = form ? validate(form) : {};
  const shownErrors = showErrors ? errors : {};
  // Ссылку проверяем сразу, а не только при сохранении: ошибка видна в предпросмотре.
  if (form && errors.promo_link_url && form.site.promo_link_url.trim().length > 1) shownErrors.promo_link_url = errors.promo_link_url;

  // Предупреждение при уходе со страницы с несохранёнными правками.
  useEffect(() => {
    if (!changed.any) return;
    const onBefore = (e: BeforeUnloadEvent): void => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', onBefore);
    return () => window.removeEventListener('beforeunload', onBefore);
  }, [changed.any]);

  const blocker = useBlocker(
    useCallback(
      ({ currentLocation, nextLocation }: { currentLocation: { pathname: string }; nextLocation: { pathname: string } }) =>
        changed.any && !saving && currentLocation.pathname !== nextLocation.pathname,
      [changed.any, saving],
    ),
  );
  const asking = useRef(false);
  useEffect(() => {
    if (blocker.state !== 'blocked' || asking.current) return;
    asking.current = true;
    void confirm({
      title: 'Уйти без сохранения?',
      text: 'Вы изменили настройки, но не сохранили их. Если уйти, правки пропадут.',
      confirmLabel: 'Уйти без сохранения',
      danger: true,
    }).then((ok) => {
      asking.current = false;
      if (ok) blocker.proceed();
      else blocker.reset();
    });
  }, [blocker, confirm]);

  const setSite = (k: SiteKey, v: string): void => setForm((f) => (f ? { ...f, site: { ...f.site, [k]: v } } : f));

  async function save(e?: FormEvent): Promise<void> {
    e?.preventDefault();
    if (!form || !initial || !changed.any) return;
    setShowErrors(true);
    if (Object.keys(errors).length > 0) {
      toast('Исправьте отмеченные поля', 'error');
      document.querySelector('.ax-field-error')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      return;
    }
    const pct = Number(form.commission.trim().replace(',', '.'));
    if (changed.commission) {
      const ok = await confirm({
        title: `Изменить комиссию с ${initial.commission}% на ${pct}%?`,
        text: 'Новая комиссия применится ко всем будущим покупкам. Уже прошедшие платежи не пересчитываются.',
        confirmLabel: 'Изменить комиссию',
      });
      if (!ok) return;
    }
    setSaving(true);
    const done: string[] = [];
    try {
      if (changed.commission) {
        const data = await updateAdminSettings(pct);
        qc.setQueryData(['admin-settings'], data);
        done.push('комиссия');
      }
      if (changed.promo) {
        const data = await updateAdminSettingsPromo(form.promoOn);
        qc.setQueryData(['admin-settings'], data);
        done.push('промо-полоса');
      }
      if (changed.site) {
        const payload = Object.fromEntries(SITE_KEYS.map((k) => [k, form.site[k].trim()])) as SiteSettings;
        const data = await adminPutSiteSettings(payload);
        qc.setQueryData(['admin-site-settings'], data);
        done.push('тексты сайта');
      }
      setShowErrors(false);
      toast('Сохранено — изменения уже на сайте');
    } catch (err) {
      toast(
        done.length ? `Сохранено частично (${done.join(', ')}). Ошибка: ${errorText(err)}` : `Не удалось сохранить: ${errorText(err)}`,
        'error',
      );
    } finally {
      setSaving(false);
      void qc.invalidateQueries({ queryKey: ['admin-settings'] });
      void qc.invalidateQueries({ queryKey: ['admin-site-settings'] });
      void qc.invalidateQueries({ queryKey: ['public-settings'] });
      void qc.invalidateQueries({ queryKey: overviewQueryKey });
    }
  }

  const loading = platform.isLoading || site.isLoading;
  const loadError = platform.error ?? site.error;

  const text = (k: SiteKey, opts?: { multiline?: boolean; type?: string; inputMode?: 'tel' | 'email' | 'url' }): JSX.Element | null => {
    if (!form) return null;
    const f = SITE_FIELDS[k];
    const v = form.site[k];
    const left = f.max - v.length;
    return (
      <Field
        label={f.label}
        error={shownErrors[k]}
        hint={
          f.hint || left < 30 ? (
            <>
              {f.hint}
              {left < 30 && <span className="asy-counter"> Осталось символов: {left}</span>}
            </>
          ) : undefined
        }
      >
        {opts?.multiline ? (
          <textarea
            className="ax-textarea asy-textarea-sm"
            value={v}
            maxLength={f.max}
            placeholder={f.def}
            onChange={(e) => setSite(k, e.target.value)}
          />
        ) : (
          <input
            className="ax-input"
            type={opts?.type ?? 'text'}
            inputMode={opts?.inputMode}
            value={v}
            maxLength={f.max}
            placeholder={f.def}
            aria-invalid={shownErrors[k] ? true : undefined}
            onChange={(e) => setSite(k, e.target.value)}
          />
        )}
      </Field>
    );
  };

  const val = (k: SiteKey): string => (form?.site[k].trim() || SITE_FIELDS[k].def);
  const phone = val('contact_phone');

  return (
    <AdminPage
      title="Настройки сайта"
      description="Комиссия платформы, жёлтая промо-полоса над сайтом и контакты в подвале. Всё, что вы сохраните, сразу появится на сайте."
    >
      {loading && <Spinner />}
      {loadError && (
        <ErrorNote
          error={loadError}
          onRetry={() => {
            void platform.refetch();
            void site.refetch();
          }}
        />
      )}

      {form && initial && (
        <form className="asy-settings" onSubmit={(e) => void save(e)} noValidate>
          <Card
            title="Платёжные настройки"
            description="Какую долю от каждой покупки курса забирает платформа. Остальное получает преподаватель."
          >
            <div className="asy-commission">
              <Field
                label="Комиссия платформы"
                error={shownErrors.commission}
                hint="Целое число от 0 до 90. Действует только на будущие покупки — прошлые платежи не пересчитываются."
              >
                <div className="asy-suffix">
                  <input
                    className="ax-input"
                    type="number"
                    min={0}
                    max={90}
                    step={1}
                    inputMode="numeric"
                    value={form.commission}
                    aria-invalid={shownErrors.commission ? true : undefined}
                    onChange={(e) => setForm({ ...form, commission: e.target.value })}
                  />
                  <span aria-hidden="true">%</span>
                </div>
              </Field>
              {!errors.commission && (
                <div className="asy-split" aria-label="Пример распределения">
                  <span className="ax-muted">Пример: курс за 10 000 ₽</span>
                  <div className="asy-split-bar" aria-hidden="true">
                    <span style={{ width: `${Number(form.commission)}%` }} />
                  </div>
                  <div className="asy-split-legend">
                    <span>
                      <i className="asy-dot asy-dot--platform" /> Платформа:{' '}
                      <b className="ax-num">{(100 * Number(form.commission)).toLocaleString('ru-RU')} ₽</b>
                    </span>
                    <span>
                      <i className="asy-dot asy-dot--teacher" /> Преподаватель:{' '}
                      <b className="ax-num">{(100 * (100 - Number(form.commission))).toLocaleString('ru-RU')} ₽</b>
                    </span>
                  </div>
                </div>
              )}
            </div>
          </Card>

          <Card
            title="Промо-полоса"
            description="Жёлтая полоса над шапкой сайта. Её видят посетители на всех страницах, кроме входа, регистрации, уроков и админки; каждый может скрыть её крестиком. Пустое поле — текст по умолчанию (виден серым)."
          >
            <div className="asy-stack">
              <Switch
                checked={form.promoOn}
                onChange={(v) => setForm({ ...form, promoOn: v })}
                label={form.promoOn ? 'Полоса показывается на сайте' : 'Полоса выключена'}
                hint="Выключите, когда акция закончится — тексты сохранятся."
              />

              <div className="asy-preview">
                <div className="asy-preview-head">
                  <span>
                    <Icon name="eye" size={16} /> Так полоса выглядит на сайте
                  </span>
                  {!form.promoOn && <span className="asy-preview-off">сейчас скрыта</span>}
                </div>
                <div className={`asy-promo-frame${form.promoOn ? '' : ' is-off'}`}>
                  <div className="promo">
                    <span>{val('promo_text')}</span>
                    <a role="presentation">{val('promo_link_text')}</a>
                    <span className="promo-close" aria-hidden="true">
                      ×
                    </span>
                  </div>
                  <div className="asy-fake-header" aria-hidden="true">
                    <span className="asy-fake-logo">S</span>
                    <span className="asy-fake-line" />
                    <span className="asy-fake-line asy-fake-line--short" />
                  </div>
                </div>
                <p className="ax-muted asy-preview-link">
                  Кнопка ведёт на: <span className="ax-mono">{val('promo_link_url')}</span>
                  {/^https:\/\//.test(form.site.promo_link_url.trim()) && ' (внешний сайт)'}
                </p>
              </div>

              <div className="ax-form-row asy-row-wide">{text('promo_text')}</div>
              <div className="asy-two">
                {text('promo_link_text')}
                {text('promo_link_url', { inputMode: 'url' })}
              </div>
            </div>
          </Card>

          <Card title="Контакты и подвал сайта" description="Показываются внизу каждой страницы. Пустое поле — значение по умолчанию (видно серым).">
            <div className="asy-stack">
              <div className="asy-three">
                {text('contact_phone', { type: 'tel', inputMode: 'tel' })}
                {text('contact_email', { type: 'email', inputMode: 'email' })}
                {text('contact_city')}
              </div>
              {text('footer_tagline', { multiline: true })}
              <div className="asy-preview">
                <div className="asy-preview-head">
                  <span>
                    <Icon name="eye" size={16} /> Так это выглядит в подвале
                  </span>
                </div>
                <div className="asy-footer-preview">
                  <span className="asy-footer-logo">
                    <span className="asy-fake-logo">S</span>skillex.space
                  </span>
                  <small>{val('footer_tagline')}</small>
                  <span className="asy-footer-contacts">
                    <b>{phone}</b>
                    <small>{val('contact_email')}</small>
                    <small>{val('contact_city')}</small>
                  </span>
                </div>
              </div>
            </div>
          </Card>

          {changed.any && (
          <div className="asy-savebar" role="region" aria-label="Несохранённые изменения">
            <span className="asy-savebar-text">
              <Icon name="alert" size={18} /> Есть несохранённые изменения
            </span>
            <div className="asy-savebar-actions">
              <button
                type="button"
                className="btn btn-ghost"
                disabled={saving}
                onClick={() => {
                  setForm(initial);
                  setShowErrors(false);
                }}
              >
                Отменить
              </button>
              <button type="submit" className="btn btn-primary" disabled={saving}>
                {saving ? 'Сохраняем…' : 'Сохранить'}
              </button>
            </div>
          </div>
          )}
        </form>
      )}
    </AdminPage>
  );
}
