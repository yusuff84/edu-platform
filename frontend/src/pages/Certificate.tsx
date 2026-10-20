// Публичная страница сертификата (T68).
// Открывается по номеру /certificates/:number без авторизации — ссылкой можно
// делиться. Данные — GET /certificates/{number}; 404 — дружелюбное состояние.
// Стили — отдельный certificate.css на переменных/классах дизайн-системы.

import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { ApiError, getCertificate } from '../api/client';
import { t } from '../i18n/t';
import { DEFAULT_DESCRIPTION, useSeo } from '../app/useSeo';
import '../styles/certificate.css';

/** Копирование в буфер обмена: Clipboard API, иначе запасной способ. */
async function copyToClipboard(value: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Нет прав/небезопасный контекст — пробуем запасной способ.
  }
  try {
    const area = document.createElement('textarea');
    area.value = value;
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    area.remove();
    return ok;
  } catch {
    return false;
  }
}

/** Дата выдачи без времени: «19.10.2026». Пустое/некорректное значение → «—». */
function formatIssuedDate(value: string): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  });
}

/** Дружелюбный 404: сертификата с таким номером нет (или номер пуст). */
function CertificateNotFound(): JSX.Element {
  return (
    <div className="wrap">
      <main className="page cert-page">
        <div className="crumbs">
          <Link to="/">{t('nav.catalog')}</Link> / <span>{t('p8.cert.pageTitle')}</span>
        </div>
        <div className="empty cert-not-found">
          <h1>{t('p8.cert.notFound')}</h1>
          <p>{t('p8.cert.notFoundHint')}</p>
          <p>
            <Link className="btn btn-primary" to="/">
              {t('error.goCatalog')}
            </Link>
          </p>
        </div>
      </main>
    </div>
  );
}

export default function Certificate(): JSX.Element {
  const { number = '' } = useParams();
  const [copied, setCopied] = useState(false);

  const query = useQuery({
    queryKey: ['certificate', number],
    queryFn: () => getCertificate(number),
    enabled: Boolean(number),
    retry: false,
  });
  useSeo(
    query.data ? `Сертификат ${query.data.number} — Edu Platform` : 'Сертификат — Edu Platform',
    query.data
      ? `Сертификат о прохождении курса «${query.data.courseTitle}» на Edu Platform.`
      : DEFAULT_DESCRIPTION,
  );

  // При переходе на другой номер (если страница не размонтировалась) —
  // подсказка о копировании не должна «переезжать» на новый сертификат.
  useEffect(() => {
    setCopied(false);
  }, [number]);

  const onCopy = useCallback(async (): Promise<void> => {
    if (!query.data) return;
    const link = `${window.location.origin}/certificates/${encodeURIComponent(
      query.data.number,
    )}`;
    setCopied(await copyToClipboard(link));
  }, [query.data]);

  if (query.isLoading) {
    return (
      <div className="wrap">
        <main className="page cert-page">
          <div className="panel" role="status">
            <p>{t('common.loading')}</p>
          </div>
        </main>
      </div>
    );
  }

  if (query.isError) {
    const status = query.error instanceof ApiError ? query.error.status : undefined;
    if (status === 404) return <CertificateNotFound />;
    return (
      <div className="wrap">
        <main className="page cert-page">
          <div className="crumbs">
            <Link to="/">{t('nav.catalog')}</Link> /{' '}
            <span>{t('p8.cert.pageTitle')}</span>
          </div>
          <div className="panel">
            <h1>{t('p8.cert.pageTitle')}</h1>
            <p className="error" role="alert">
              {query.error instanceof ApiError
                ? query.error.message
                : t('error.network')}
            </p>
            <button type="button" className="btn" onClick={() => void query.refetch()}>
              {t('common.retry')}
            </button>
          </div>
        </main>
      </div>
    );
  }

  const cert = query.data;
  if (!cert) return <CertificateNotFound />;

  // ФИО может быть пустым (displayName не задан) — показываем нейтральное «Студент».
  const studentName = cert.studentName?.trim() || t('p8.cert.student');

  return (
    <div className="wrap">
      <main className="page cert-page">
        <div className="crumbs">
          <Link to="/">{t('nav.catalog')}</Link> / <span>{t('p8.cert.pageTitle')}</span>
        </div>
        <article className="panel cert-card">
          <div className="cert-seal" aria-hidden="true">
            <span className="cert-seal-mark">E</span>
            <span className="cert-seal-text">Edu Platform</span>
          </div>
          <p className="cert-kicker">{t('p8.cert.awarded')}</p>
          <h1 className="cert-title">{t('p8.cert.pageTitle')}</h1>
          <dl className="cert-facts">
            <div className="cert-fact">
              <dt>{t('p8.cert.student')}</dt>
              <dd>{studentName}</dd>
            </div>
            <div className="cert-fact">
              <dt>{t('p8.cert.course')}</dt>
              <dd>{cert.courseTitle}</dd>
            </div>
            <div className="cert-fact">
              <dt>{t('p8.cert.issuedAt')}</dt>
              <dd>{formatIssuedDate(cert.issuedAt)}</dd>
            </div>
            <div className="cert-fact">
              <dt>{t('p8.cert.number')}</dt>
              <dd>
                <span className="cert-number">{cert.number}</span>
              </dd>
            </div>
          </dl>
          <div className="cert-actions">
            <button type="button" className="btn btn-primary" onClick={() => void onCopy()}>
              {t('p8.cert.copyLink')}
            </button>
            <Link className="btn" to={`/courses/${cert.courseSlug}`}>
              {t('p8.cert.toCourse')}
            </Link>
            <Link className="btn btn-ghost" to="/">
              {t('error.goCatalog')}
            </Link>
            <span className="cert-copied" role="status" aria-live="polite">
              {copied ? t('p8.cert.copied') : ''}
            </span>
          </div>
        </article>
      </main>
    </div>
  );
}
