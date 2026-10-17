// T29: раздел «Ученики курса» в кабинете учителя.
// Это ЗАГЛУШКА КАРКАСА — файл полностью принадлежит задаче T29.
// Разводка (роут /teach/:id/students, ссылка из конструктора) уже сделана
// координатором, чтобы задачи не конфликтовали между собой.
import { t } from '../i18n/t';

export default function CourseStudents(): JSX.Element {
  return (
    <div className="wrap">
      <div className="page">
        <h1>{t('courseStudents.title')}</h1>
        <div className="panel">
          <p className="muted">{t('common.inDevelopment')}</p>
        </div>
      </div>
    </div>
  );
}
