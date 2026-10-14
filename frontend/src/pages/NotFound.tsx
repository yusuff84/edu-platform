import { Link } from 'react-router-dom';
import { t } from '../i18n/t';
import '../styles/shell.css';

/**
 * 404 для неизвестных URL (SPA — HTTP-кода нет, текст и ссылка обязательны).
 */
export default function NotFound(): JSX.Element {
  return (
    <main className="shell-center">
      <h1>{t('error.notFound')}</h1>
      <p>{t('error.notFoundHint')}</p>
      <p>
        <Link to="/">{t('error.goCatalog')}</Link>
      </p>
    </main>
  );
}
