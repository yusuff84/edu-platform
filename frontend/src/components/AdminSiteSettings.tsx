import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminGetSiteSettings, adminPutSiteSettings } from '../api/adminOps';
import type { SiteSettings } from '../api/adminOps';

type Key = keyof SiteSettings;
const FIELDS: Array<{ key: Key; label: string; placeholder: string; max: number }> = [
  { key: 'promo_text', label: 'Текст промо-полосы', placeholder: 'Начните учиться бесплатно — регистрация займёт минуту', max: 160 },
  { key: 'promo_link_text', label: 'Кнопка промо-полосы', placeholder: 'Выбрать курс', max: 40 },
  { key: 'promo_link_url', label: 'Ссылка кнопки (/путь или https://…)', placeholder: '/', max: 300 },
  { key: 'contact_phone', label: 'Телефон в футере', placeholder: '8 (800) 600-78-47', max: 40 },
  { key: 'contact_email', label: 'Email в футере', placeholder: 'hello@edu-platform.ru', max: 120 },
  { key: 'contact_city', label: 'Город в футере', placeholder: 'Москва', max: 80 },
  { key: 'footer_tagline', label: 'Описание в футере', placeholder: 'Маркетплейс IT-курсов для начинающих…', max: 300 },
];

/** Тексты сайта: промо-полоса и контакты в футере. Пустое поле — текст по умолчанию. */
export function AdminSiteSettings(): JSX.Element {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ['admin-site-settings'], queryFn: adminGetSiteSettings });
  const [values, setValues] = useState<SiteSettings>({});
  useEffect(() => {
    if (query.data) setValues(query.data.settings);
  }, [query.data]);
  const save = useMutation({
    mutationFn: () =>
      adminPutSiteSettings(Object.fromEntries(FIELDS.map((f) => [f.key, values[f.key] ?? ''])) as SiteSettings),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-site-settings'] });
      void qc.invalidateQueries({ queryKey: ['public-settings'] });
    },
  });
  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    save.mutate();
  }
  return (
    <form className="aua-form site-settings-form" onSubmit={onSubmit}>
      <h3>Тексты сайта</h3>
      <p className="muted">Пустое поле — используется текст по умолчанию (показан серым).</p>
      {FIELDS.map((f) => (
        <label key={f.key}>
          <span>{f.label}</span>
          <input
            className="input"
            value={values[f.key] ?? ''}
            maxLength={f.max}
            placeholder={f.placeholder}
            onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}
          />
        </label>
      ))}
      <button type="submit" className="btn btn-sm btn-primary" disabled={save.isPending || query.isLoading}>
        {save.isPending ? 'Сохраняем…' : 'Сохранить тексты'}
      </button>
      {save.isSuccess && <p className="aua-note" role="status">Сохранено — изменения уже на сайте.</p>}
      {save.isError && <p className="error" role="alert">{save.error instanceof Error ? save.error.message : 'Ошибка'}</p>}
    </form>
  );
}
