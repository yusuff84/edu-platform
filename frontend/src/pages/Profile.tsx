// T30: личный кабинет ученика — профиль, прогресс, история платежей.
// Это ЗАГЛУШКА КАРКАСА — файл полностью принадлежит задаче T30.
// Разводка (роут /profile, ссылка в шапке) уже сделана координатором,
// чтобы задачи не конфликтовали между собой.
import { t } from '../i18n/t';

export default function Profile(): JSX.Element {
  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('profile.title')}</h1>
        <div className="panel">
          <p className="muted">{t('common.inDevelopment')}</p>
        </div>
      </div>
    </div>
  );
}
