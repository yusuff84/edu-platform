// T27: раздел «Сводка» (дашборд) супер-админки.
// Это ЗАГЛУШКА КАРКАСА — файл полностью принадлежит задаче T27.
// Разводка (роут /admin/stats, ссылка в навигации админки) уже сделана
// координатором, чтобы задачи не конфликтовали между собой.
import { t } from '../i18n/t';

export default function AdminStats(): JSX.Element {
  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('adminStats.title')}</h1>
        <div className="panel">
          <p className="muted">{t('common.inDevelopment')}</p>
        </div>
      </div>
    </div>
  );
}
