// T26: раздел «Пользователи» супер-админки.
// Это ЗАГЛУШКА КАРКАСА — файл полностью принадлежит задаче T26.
// Разводка (роут /admin/users, ссылка в навигации админки) уже сделана
// координатором, чтобы задачи не конфликтовали между собой.
import { t } from '../i18n/t';

export default function AdminUsers(): JSX.Element {
  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('adminUsers.title')}</h1>
        <div className="panel">
          <p className="muted">{t('common.inDevelopment')}</p>
        </div>
      </div>
    </div>
  );
}
