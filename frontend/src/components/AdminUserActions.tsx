import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  adminCompleteCourse,
  adminIssueCertificate,
  adminLogoutUser,
  adminResetProgress,
  adminRevokeCertificate,
  adminSetUserPassword,
  adminUpdateUserProfile,
  adminUserCourses,
} from '../api/adminOps';
import type { AdminUserCourse } from '../api/adminOps';
import { grantEnrollment, revokeEnrollment, unrevokeEnrollment } from '../api/adminEnrollments';
import { listAdminCourses } from '../api/adminConsole';

const STATUS_LABEL: Record<string, string> = { active: 'Доступ открыт', revoked: 'Доступ отозван', refunded: 'Возврат' };

function errText(e: unknown): string {
  return e instanceof Error ? e.message : 'Что-то пошло не так';
}

/**
 * Действия администратора в карточке пользователя: профиль, пароль, сеансы,
 * курсы (выдать, засчитать, сбросить прогресс, отозвать/вернуть доступ),
 * сертификаты (выдать, открыть, отозвать).
 */
export function AdminUserActions({
  user,
  onChanged,
}: {
  user: { id: string; email: string; displayName?: string };
  onChanged: () => void;
}): JSX.Element {
  const qc = useQueryClient();
  const [email, setEmail] = useState(user.email);
  const [name, setName] = useState(user.displayName ?? '');
  const [newPassword, setNewPassword] = useState('');
  const [shownPassword, setShownPassword] = useState('');
  const [grantCourse, setGrantCourse] = useState('');
  const [note, setNote] = useState('');

  const courses = useQuery({ queryKey: ['admin-user-courses', user.id], queryFn: () => adminUserCourses(user.id) });
  const allCourses = useQuery({ queryKey: ['admin-ops-course-options'], queryFn: () => listAdminCourses({ limit: 100 }) });
  const refresh = (): void => {
    void qc.invalidateQueries({ queryKey: ['admin-user-courses', user.id] });
    void qc.invalidateQueries({ queryKey: ['admin-enrollments'] });
    void qc.invalidateQueries({ queryKey: ['admin-user-enrollments', user.id] });
  };

  const run = useMutation({
    mutationFn: async (action: () => Promise<string>) => action(),
    onSuccess: (msg) => {
      setNote(msg);
      refresh();
    },
    onError: (e) => setNote(errText(e)),
  });

  const confirmRun = (question: string, action: () => Promise<string>): void => {
    if (window.confirm(question)) run.mutate(action);
  };

  function onSaveProfile(e: FormEvent): void {
    e.preventDefault();
    run.mutate(async () => {
      await adminUpdateUserProfile(user.id, { email: email.trim(), displayName: name });
      onChanged();
      return 'Профиль сохранён';
    });
  }

  function onSetPassword(e: FormEvent): void {
    e.preventDefault();
    run.mutate(async () => {
      const res = await adminSetUserPassword(user.id, newPassword || undefined);
      setShownPassword(res.password);
      setNewPassword('');
      return res.password
        ? 'Новый пароль сгенерирован — передайте его пользователю. Все его сеансы завершены.'
        : 'Пароль изменён. Все сеансы пользователя завершены.';
    });
  }

  const owned = new Set((courses.data?.items ?? []).map((c) => c.courseId));
  const grantOptions = (allCourses.data?.items ?? []).filter((c) => c.status !== 'draft' && !owned.has(c.id));

  const courseRow = (c: AdminUserCourse): JSX.Element => {
    const pct = c.totalLessons ? Math.round((c.completedLessons / c.totalLessons) * 100) : 0;
    return (
      <li key={c.courseId} className="aua-course">
        <div className="aua-course-head">
          <b>{c.courseTitle}</b>
          <span className={`aua-status aua-status--${c.status}`}>{STATUS_LABEL[c.status] ?? c.status}</span>
        </div>
        <div className="aua-progress" aria-label={`Прогресс ${pct}%`}>
          <i style={{ width: `${pct}%` }} />
        </div>
        <span className="muted aua-meta">
          Пройдено {c.completedLessons} из {c.totalLessons} ({pct}%)
          {c.certificateNumber && (
            <>
              {' · '}
              <Link to={`/certificates/${encodeURIComponent(c.certificateNumber)}`} target="_blank">
                сертификат {c.certificateNumber}
              </Link>
            </>
          )}
        </span>
        <div className="aua-actions">
          <button type="button" className="btn btn-sm" disabled={run.isPending}
            onClick={() => confirmRun(`Засчитать весь курс «${c.courseTitle}» и выдать сертификат?`, async () => {
              const r = await adminCompleteCourse(user.id, c.courseId);
              return `Курс засчитан, сертификат ${r.certificateNumber}`;
            })}>
            Засчитать курс
          </button>
          {!c.certificateNumber && (
            <button type="button" className="btn btn-sm" disabled={run.isPending}
              onClick={() => run.mutate(async () => {
                const r = await adminIssueCertificate({ studentId: user.id, courseId: c.courseId });
                return `Сертификат выдан: ${r.certificateNumber}`;
              })}>
              Выдать сертификат
            </button>
          )}
          {c.certificateNumber && (
            <button type="button" className="btn btn-sm" disabled={run.isPending}
              onClick={() => confirmRun(`Отозвать сертификат ${c.certificateNumber}? Ссылка на него перестанет работать.`, async () => {
                await adminRevokeCertificate(c.certificateNumber);
                return 'Сертификат отозван';
              })}>
              Отозвать сертификат
            </button>
          )}
          <button type="button" className="btn btn-sm" disabled={run.isPending}
            onClick={() => confirmRun(`Сбросить прогресс по курсу «${c.courseTitle}»?`, async () => {
              const r = await adminResetProgress(user.id, c.courseId);
              return `Прогресс сброшен (${r.deleted} уроков)`;
            })}>
            Сбросить прогресс
          </button>
          {c.status === 'active' ? (
            <button type="button" className="btn btn-sm aua-danger" disabled={run.isPending}
              onClick={() => confirmRun(`Отозвать доступ к курсу «${c.courseTitle}»?`, async () => {
                await revokeEnrollment(user.id, c.courseId);
                return 'Доступ отозван';
              })}>
              Отозвать доступ
            </button>
          ) : (
            <button type="button" className="btn btn-sm" disabled={run.isPending}
              onClick={() => run.mutate(async () => {
                await unrevokeEnrollment(user.id, c.courseId);
                return 'Доступ возвращён';
              })}>
              Вернуть доступ
            </button>
          )}
        </div>
      </li>
    );
  };

  return (
    <div className="aua">
      {note && (
        <p className="aua-note" role="status">
          {note}
        </p>
      )}
      {shownPassword && (
        <p className="aua-password" role="status">
          Новый пароль: <code>{shownPassword}</code>{' '}
          <button type="button" className="btn btn-sm" onClick={() => void navigator.clipboard?.writeText(shownPassword)}>
            Скопировать
          </button>
        </p>
      )}

      <section className="aua-section">
        <h3>Курсы и сертификаты</h3>
        {courses.isLoading && <p className="muted">Загрузка…</p>}
        {courses.data && courses.data.items.length === 0 && <p className="muted">Пользователь не записан ни на один курс.</p>}
        <ul className="aua-courses">{(courses.data?.items ?? []).map(courseRow)}</ul>
        <form
          className="aua-inline"
          onSubmit={(e) => {
            e.preventDefault();
            if (!grantCourse) return;
            run.mutate(async () => {
              const r = await grantEnrollment(user.email, grantCourse);
              setGrantCourse('');
              return `Доступ к курсу «${r.courseTitle}» открыт`;
            });
          }}
        >
          <select className="input" value={grantCourse} onChange={(e) => setGrantCourse(e.target.value)} aria-label="Курс для выдачи">
            <option value="">Выдать доступ к курсу…</option>
            {grantOptions.map((c) => (
              <option key={c.id} value={c.id}>
                {c.title}
              </option>
            ))}
          </select>
          <button type="submit" className="btn btn-sm btn-primary" disabled={!grantCourse || run.isPending}>
            Выдать
          </button>
        </form>
      </section>

      <section className="aua-section">
        <h3>Профиль</h3>
        <form className="aua-form" onSubmit={onSaveProfile}>
          <label>
            <span>Email</span>
            <input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label>
            <span>Имя (так оно будет в сертификате)</span>
            <input className="input" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
          </label>
          <button type="submit" className="btn btn-sm" disabled={run.isPending}>
            Сохранить профиль
          </button>
        </form>
      </section>

      <section className="aua-section">
        <h3>Доступ к аккаунту</h3>
        <form className="aua-inline" onSubmit={onSetPassword}>
          <input
            className="input"
            type="text"
            autoComplete="off"
            placeholder="Новый пароль (пусто — сгенерировать)"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            aria-label="Новый пароль"
          />
          <button type="submit" className="btn btn-sm" disabled={run.isPending}>
            Сменить пароль
          </button>
        </form>
        <button
          type="button"
          className="btn btn-sm"
          disabled={run.isPending}
          onClick={() => confirmRun('Завершить все сеансы пользователя? Ему придётся войти заново.', async () => {
            await adminLogoutUser(user.id);
            return 'Все сеансы пользователя завершены';
          })}
        >
          Выйти на всех устройствах
        </button>
      </section>
    </div>
  );
}
