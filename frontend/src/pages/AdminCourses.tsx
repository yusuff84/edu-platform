// T27: раздел «Курсы» супер-админки.
// Это ЗАГЛУШКА КАРКАСА — файл полностью принадлежит задаче T27.
// Разводка (роут /admin/courses, ссылка в навигации админки) уже сделана
// координатором, чтобы задачи не конфликтовали между собой.
import { t } from '../i18n/t';

export default function AdminCourses(): JSX.Element {
  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('adminCourses.title')}</h1>
        <div className="panel">
          <p className="muted">{t('common.inDevelopment')}</p>
        </div>
      </div>
    </div>
  );
}
