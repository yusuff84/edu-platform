import { useEffect, useState } from 'react';
import QRCode from 'qrcode';
import '../styles/certdoc.css';

interface CertificateDocumentProps {
  number: string;
  studentName: string;
  courseTitle: string;
  issuedAt: string;
  verifyUrl: string;
}

function formatLongDate(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
}

/**
 * Сертификат платформы skillex.space — документ в пропорциях A4 (альбомный).
 * Размеры шрифтов заданы в единицах контейнера (cqw), поэтому документ
 * одинаково выглядит на телефоне, на десктопе и при печати / сохранении в PDF.
 * QR-код ведёт на публичную страницу проверки сертификата.
 */
export function CertificateDocument({
  number,
  studentName,
  courseTitle,
  issuedAt,
  verifyUrl,
}: CertificateDocumentProps): JSX.Element {
  const [qr, setQr] = useState('');
  useEffect(() => {
    let alive = true;
    QRCode.toString(verifyUrl, { type: 'svg', margin: 0, errorCorrectionLevel: 'M', color: { dark: '#141414', light: '#0000' } })
      .then((svg) => {
        if (alive) setQr(svg);
      })
      .catch(() => setQr(''));
    return () => {
      alive = false;
    };
  }, [verifyUrl]);

  return (
    <div className="certdoc-frame">
      <article className="certdoc" aria-label={`Сертификат ${number}`}>
        <div className="certdoc-border" aria-hidden="true" />
        <header className="certdoc-head">
          <span className="certdoc-logo">
            <span className="certdoc-logo-mark" aria-hidden="true">S</span>
            skillex.space
          </span>
          <span className="certdoc-number">№ {number}</span>
        </header>

        <div className="certdoc-body">
          <p className="certdoc-kicker">Сертификат</p>
          <h1 className="certdoc-title">о прохождении онлайн-курса</h1>
          <p className="certdoc-label">Настоящим подтверждается, что</p>
          <p className="certdoc-name">{studentName}</p>
          <p className="certdoc-label">успешно завершил(а) все уроки и тесты курса</p>
          <p className="certdoc-course">«{courseTitle}»</p>
        </div>

        <footer className="certdoc-foot">
          <div className="certdoc-meta">
            <span className="certdoc-meta-label">Дата выдачи</span>
            <span className="certdoc-meta-value">{formatLongDate(issuedAt)}</span>
          </div>
          <div className="certdoc-sign">
            <span className="certdoc-sign-line" aria-hidden="true" />
            <span className="certdoc-meta-label">Образовательная платформа skillex.space</span>
          </div>
          <div className="certdoc-verify">
            {qr ? (
              <span className="certdoc-qr" aria-hidden="true" dangerouslySetInnerHTML={{ __html: qr }} />
            ) : (
              <span className="certdoc-qr" aria-hidden="true" />
            )}
            <span className="certdoc-verify-text">
              Проверить подлинность:
              <br />
              {verifyUrl.replace(/^https?:\/\//, '')}
            </span>
          </div>
        </footer>
      </article>
    </div>
  );
}
