import { useState } from 'react';
import type { FormEvent } from 'react';
import { useMutation } from '@tanstack/react-query';
import { grantEnrollment } from '../api/adminEnrollments';
import type { AdminCourseRow } from '../api/adminConsole';
import { formatPrice } from '../api/client';

/**
 * Ручная выдача доступа к курсу: email зарегистрированного пользователя +
 * курс. Работает и для платных курсов (без оплаты и без записи в журнале
 * платежей), отозванный доступ восстанавливает.
 */
export function AdminGrantAccess({
  courses,
  onGranted,
}: {
  courses: AdminCourseRow[];
  onGranted: () => void;
}): JSX.Element {
  const [email, setEmail] = useState('');
  const [courseId, setCourseId] = useState('');
  const grant = useMutation({
    mutationFn: () => grantEnrollment(email, courseId),
    onSuccess: () => onGranted(),
  });
  // Черновику доступ не выдать (бэкенд отвечает 400) — в списке их нет.
  const options = courses
    .filter((c) => c.status !== 'draft')
    .sort((a, b) => a.title.localeCompare(b.title, 'ru'));
  const canSubmit = email.includes('@') && courseId !== '' && !grant.isPending;

  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    if (canSubmit) grant.mutate();
  }

  const res = grant.data;
  return (
    <section className="panel ae-grant" aria-labelledby="ae-grant-title">
      <h2 id="ae-grant-title">Выдать доступ к курсу</h2>
      <p className="muted ae-grant-hint">
        Откроет курс пользователю без оплаты — в том числе платный. Пользователь должен быть
        зарегистрирован. В журнал платежей и выручку такая запись не попадает.
      </p>
      <form className="ae-grant-form" onSubmit={onSubmit}>
        <label className="ae-field ae-grant-email" htmlFor="ae-grant-email">
          <span>Email пользователя</span>
          <input
            id="ae-grant-email"
            className="input"
            type="email"
            autoComplete="off"
            placeholder="user@example.com"
            value={email}
            onChange={(e) => {
              setEmail(e.target.value);
              grant.reset();
            }}
          />
        </label>
        <label className="ae-field ae-grant-course" htmlFor="ae-grant-course">
          <span>Курс</span>
          <select
            id="ae-grant-course"
            className="input"
            value={courseId}
            onChange={(e) => {
              setCourseId(e.target.value);
              grant.reset();
            }}
          >
            <option value="">Выберите курс</option>
            {options.map((c) => (
              <option key={c.id} value={c.id}>
                {c.title} · {c.priceCents > 0 ? formatPrice(c.priceCents) : 'бесплатно'}
                {c.status === 'archived' ? ' · в архиве' : ''}
              </option>
            ))}
          </select>
        </label>
        <button type="submit" className="btn btn-primary" disabled={!canSubmit}>
          {grant.isPending ? 'Выдаём…' : 'Выдать доступ'}
        </button>
      </form>
      {grant.isError && (
        <p className="error" role="alert">
          {grant.error instanceof Error ? grant.error.message : 'Не удалось выдать доступ'}
        </p>
      )}
      {res && (
        <p className="ae-grant-ok" role="status">
          {res.wasActive
            ? `У ${res.email} уже есть доступ к курсу «${res.courseTitle}».`
            : `Готово: ${res.email} получил доступ к курсу «${res.courseTitle}».`}
        </p>
      )}
    </section>
  );
}
