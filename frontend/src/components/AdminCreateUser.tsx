import { useState } from 'react';
import type { FormEvent } from 'react';
import { useMutation } from '@tanstack/react-query';
import { adminCreateUser } from '../api/adminOps';
import type { AdminRole } from '../api/adminOps';

/** Создание пользователя вручную (без письма): пароль задаётся или генерируется. */
export function AdminCreateUser({ onCreated }: { onCreated: () => void }): JSX.Element {
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [role, setRole] = useState<AdminRole>('student');
  const [password, setPassword] = useState('');
  const create = useMutation({
    mutationFn: () => adminCreateUser({ email: email.trim(), role, displayName: name.trim() || undefined, password: password || undefined }),
    onSuccess: () => {
      setEmail('');
      setName('');
      setPassword('');
      onCreated();
    },
  });
  function onSubmit(e: FormEvent): void {
    e.preventDefault();
    if (email.includes('@')) create.mutate();
  }
  return (
    <section className="panel au-create">
      <button type="button" className="btn btn-sm btn-primary" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
        {open ? 'Скрыть' : '+ Создать пользователя'}
      </button>
      {open && (
        <form className="aua-form au-create-form" onSubmit={onSubmit}>
          <label>
            <span>Email</span>
            <input className="input" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label>
            <span>Имя</span>
            <input className="input" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
          </label>
          <label>
            <span>Роль</span>
            <select className="input" value={role} onChange={(e) => setRole(e.target.value as AdminRole)}>
              <option value="student">Ученик</option>
              <option value="teacher">Преподаватель</option>
              <option value="admin">Администратор</option>
            </select>
          </label>
          <label>
            <span>Пароль (пусто — сгенерировать)</span>
            <input className="input" type="text" autoComplete="off" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          <button type="submit" className="btn btn-sm btn-primary" disabled={create.isPending}>
            {create.isPending ? 'Создаём…' : 'Создать'}
          </button>
        </form>
      )}
      {create.isError && <p className="error" role="alert">{create.error instanceof Error ? create.error.message : 'Ошибка'}</p>}
      {create.data && (
        <p className="aua-password" role="status">
          Создан {create.data.user.email}.
          {create.data.password ? (
            <>
              {' '}Пароль: <code>{create.data.password}</code> — передайте его пользователю, повторно он не показывается.
            </>
          ) : null}
        </p>
      )}
    </section>
  );
}
