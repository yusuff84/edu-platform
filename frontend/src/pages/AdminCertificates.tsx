import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminIssueCertificate, adminListCertificates, adminRevokeCertificate } from '../api/adminOps';
import { listAdminCourses } from '../api/adminConsole';
import { formatDate } from '../api/client';
import { useSeo } from '../app/useSeo';
import '../styles/adminUsers.css';

/** Админка: все сертификаты платформы — поиск, выдача вручную, отзыв. */
export default function AdminCertificates(): JSX.Element {
  useSeo('Сертификаты — админка skillex.space');
  const qc = useQueryClient();
  const [search, setSearch] = useState('');
  const [q, setQ] = useState('');
  const [email, setEmail] = useState('');
  const [courseId, setCourseId] = useState('');
  const [note, setNote] = useState('');

  useEffect(() => {
    const id = window.setTimeout(() => setQ(search.trim()), 300);
    return () => window.clearTimeout(id);
  }, [search]);

  const list = useQuery({ queryKey: ['admin-certificates', q], queryFn: () => adminListCertificates(q) });
  const courses = useQuery({ queryKey: ['admin-ops-course-options'], queryFn: () => listAdminCourses({ limit: 100 }) });
  const refresh = (): void => void qc.invalidateQueries({ queryKey: ['admin-certificates'] });

  const issue = useMutation({
    mutationFn: () => adminIssueCertificate({ email: email.trim(), courseId }),
    onSuccess: (r) => {
      setNote(`Сертификат выдан: ${r.certificateNumber}`);
      setEmail('');
      refresh();
    },
    onError: (e) => setNote(e instanceof Error ? e.message : 'Ошибка'),
  });
  const revoke = useMutation({
    mutationFn: (number: string) => adminRevokeCertificate(number),
    onSuccess: () => {
      setNote('Сертификат отозван');
      refresh();
    },
    onError: (e) => setNote(e instanceof Error ? e.message : 'Ошибка'),
  });

  function onIssue(e: FormEvent): void {
    e.preventDefault();
    if (email.includes('@') && courseId) issue.mutate();
  }

  const items = list.data?.items ?? [];
  return (
    <div className="wrap">
      <div className="page">
        <main className="au-admin">
          <p className="au-back">
            <Link to="/admin">Кабинет администратора</Link>
          </p>
          <h1>Сертификаты</h1>

          <section className="panel aua-section">
            <h2>Выдать сертификат вручную</h2>
            <p className="muted">
              Например, если курс пройден вне платформы. Пользователь должен быть зарегистрирован; доступ к курсу откроется автоматически.
            </p>
            <form className="aua-form au-create-form" onSubmit={onIssue}>
              <label>
                <span>Email пользователя</span>
                <input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
              </label>
              <label>
                <span>Курс</span>
                <select className="input" value={courseId} onChange={(e) => setCourseId(e.target.value)}>
                  <option value="">Выберите курс</option>
                  {(courses.data?.items ?? []).map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.title}
                    </option>
                  ))}
                </select>
              </label>
              <button type="submit" className="btn btn-sm btn-primary" disabled={!email.includes('@') || !courseId || issue.isPending}>
                Выдать сертификат
              </button>
            </form>
            {note && <p className="aua-note" role="status">{note}</p>}
          </section>

          <section className="panel">
            <label className="au-field au-field--search" htmlFor="cert-search">
              <span>Поиск по email, имени, номеру или курсу</span>
              <input id="cert-search" className="input" value={search} onChange={(e) => setSearch(e.target.value)} />
            </label>
            <p className="muted">Всего: {list.data?.total ?? '…'}</p>
            {items.length === 0 && list.data && <p className="muted">Сертификатов не найдено.</p>}
            <ul className="aua-courses">
              {items.map((c) => (
                <li key={c.number} className="aua-course">
                  <div className="aua-course-head">
                    <Link to={`/certificates/${encodeURIComponent(c.number)}`} target="_blank">
                      <b>{c.number}</b>
                    </Link>
                    <span className="muted">{formatDate(c.issuedAt)}</span>
                  </div>
                  <span>
                    {c.studentName || 'Имя не указано'} · {c.email}
                  </span>
                  <span className="muted">{c.courseTitle}</span>
                  <div className="aua-actions">
                    <button
                      type="button"
                      className="btn btn-sm aua-danger"
                      disabled={revoke.isPending}
                      onClick={() => {
                        if (window.confirm(`Отозвать сертификат ${c.number}? Ссылка на него перестанет работать.`)) revoke.mutate(c.number);
                      }}
                    >
                      Отозвать
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          </section>
        </main>
      </div>
    </div>
  );
}
