// Настройки платформы — отдельная страница /admin/settings.
// Логика и разметка перенесены из секции #settings страницы Admin.tsx:
// комиссия платформы (US6) и тогл промо-баннера (пост-P7).
import { type FormEvent, useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { getAdminSettings, updateAdminSettings, updateAdminSettingsPromo } from '../api/client';
import { t } from '../i18n/t';
import { useSeo } from '../app/useSeo';
import { errorText } from './adminShared';
// Стили страницы изолированы в styles/admin.css — общий styles.css не трогаем.
import '../styles/admin.css';

export default function AdminSettings(): JSX.Element {
  useSeo(`${t('admin.settingsTitle')} — ${t('cabinet.adminTitle')} — Edu Platform`);
  const qc = useQueryClient();

  const settings = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getAdminSettings,
    retry: false,
  });

  // ---- Комиссия платформы ----
  const [commissionInput, setCommissionInput] = useState('');
  const [commissionMsg, setCommissionMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    if (typeof settings.data?.commissionPct === 'number') {
      setCommissionInput(String(settings.data.commissionPct));
    }
  }, [settings.data]);

  const commissionMutation = useMutation({
    mutationFn: (pct: number) => updateAdminSettings(pct),
    onSuccess: (data) => {
      qc.setQueryData(['admin-settings'], data);
      setCommissionInput(String(data.commissionPct));
      setCommissionMsg({ ok: true, text: t('admin.commissionSaved') });
    },
    onError: (e: unknown) => setCommissionMsg({ ok: false, text: errorText(e) }),
  });

  // ---- Пост-P7: промо-баннер (тогл) ----
  // Отдельная мутация: PUT частичный (только promoBarEnabled), комиссию не
  // задевает. После успеха обновляем и ['admin-settings'], и ['public-settings']
  // — Layout показывает/скрывает полосу сразу, без перезагрузки страницы.
  const [promoMsg, setPromoMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const promoMutation = useMutation({
    mutationFn: (enabled: boolean) => updateAdminSettingsPromo(enabled),
    onSuccess: (data) => {
      qc.setQueryData(['admin-settings'], data);
      void qc.invalidateQueries({ queryKey: ['public-settings'] });
      setPromoMsg({ ok: true, text: t('admin.promoSaved') });
    },
    onError: (e: unknown) => setPromoMsg({ ok: false, text: errorText(e) }),
  });

  function onSaveCommission(e: FormEvent<HTMLFormElement>): void {
    e.preventDefault();
    const raw = commissionInput.trim().replace(',', '.');
    const pct = raw === '' ? NaN : Number(raw);
    if (!Number.isInteger(pct) || pct < 0 || pct > 90) {
      setCommissionMsg({ ok: false, text: t('admin.commissionInvalid') });
      return;
    }
    const confirmed = window.confirm(
      `${t('admin.commissionConfirm')}\n${t('admin.commissionHint')}`,
    );
    if (!confirmed) return;
    setCommissionMsg(null);
    commissionMutation.mutate(pct);
  }

  return (
    <div className="wrap">
      <div className="page">
        <main className="admin">
          <p className="muted">
            <Link to="/admin">{t('p10.nav.backToAdmin')}</Link>
          </p>
          <h1>{t('admin.settingsTitle')}</h1>

          <section className="admin-section panel" aria-label={t('admin.settingsTitle')}>
            {settings.isLoading && <p>{t('admin.loading')}</p>}
            {settings.isError && (
              <div className="admin-error" role="alert">
                <p className="error">{errorText(settings.error)}</p>
                <button type="button" className="btn" onClick={() => void settings.refetch()}>
                  {t('common.retry')}
                </button>
              </div>
            )}

            <form className="form-grid" onSubmit={onSaveCommission}>
              <label htmlFor="admin-commission">{t('admin.commissionPct')}</label>
              <input
                id="admin-commission"
                className="input"
                type="number"
                min={0}
                max={90}
                step={1}
                inputMode="numeric"
                value={commissionInput}
                onChange={(e) => setCommissionInput(e.target.value)}
              />
              <p className="muted">{t('admin.commissionHint')}</p>
              {commissionMsg && (
                <p
                  role={commissionMsg.ok ? 'status' : 'alert'}
                  className={commissionMsg.ok ? '' : 'error'}
                >
                  {commissionMsg.text}
                </p>
              )}
              <button
                type="submit"
                className="btn btn-primary"
                disabled={commissionMutation.isPending || settings.isLoading}
              >
                {commissionMutation.isPending ? t('admin.loading') : t('admin.save')}
              </button>
            </form>

            {/* Пост-P7: тогл промо-баннера — общий флаг для шапки всех страниц. */}
            <div className="admin-promo">
              <label htmlFor="admin-promo-bar">
                <input
                  id="admin-promo-bar"
                  type="checkbox"
                  checked={settings.data?.promoBarEnabled ?? false}
                  disabled={settings.isLoading || promoMutation.isPending}
                  onChange={(e) => promoMutation.mutate(e.target.checked)}
                />
                <span>{t('admin.promoBar')}</span>
              </label>
              <p className="muted">{t('admin.promoBarHint')}</p>
              {promoMsg && (
                <p
                  role={promoMsg.ok ? 'status' : 'alert'}
                  className={promoMsg.ok ? '' : 'error'}
                >
                  {promoMsg.text}
                </p>
              )}
            </div>
          </section>
        </main>
      </div>
    </div>
  );
}
